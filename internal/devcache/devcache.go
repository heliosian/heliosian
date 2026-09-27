package devcache

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	"heliosian/internal/intercept"
)

const (
	dir        = "local/cache/blobs"
	host       = "storage.googleapis.com"
	objectPath = "/storage/v1/b/"
)

func Install() {
	intercept.Install(host, &proxy{dir: dir, upstream: http.DefaultTransport.(*http.Transport).Clone()})
	slog.Info("devcache: caching storage reads", "dir", dir)
}

type proxy struct {
	dir      string
	upstream http.RoundTripper
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resp := p.answer(r)
	defer resp.Body.Close()
	maps.Copy(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (p *proxy) answer(req *http.Request) *http.Response {
	path, named := p.cachePath(req.URL.Path)
	cacheable := named && req.Method == http.MethodGet && req.URL.Query().Get("alt") == "media"
	if cacheable {
		resp, err := p.cached(req, path)
		if err != nil {
			return failed(req, err)
		}
		if resp != nil {
			return resp
		}
	}
	resp, err := p.forward(req)
	if err != nil {
		return failed(req, err)
	}
	if cacheable && resp.StatusCode == http.StatusOK {
		if err := p.store(path, resp); err != nil {
			return failed(req, err)
		}
	}
	if named && req.Method == http.MethodDelete {
		if err := p.forget(path); err != nil {
			return failed(req, err)
		}
	}
	return resp
}

func (p *proxy) cachePath(urlPath string) (string, bool) {
	rest, ok := strings.CutPrefix(urlPath, objectPath)
	if !ok {
		return "", false
	}
	bucket, object, ok := strings.Cut(rest, "/o/")
	if !ok || bucket == "" || object == "" {
		return "", false
	}
	root := filepath.Clean(p.dir)
	path := filepath.Join(root, bucket, filepath.FromSlash(object))
	if !strings.HasPrefix(path, root+string(filepath.Separator)) {
		return "", false
	}
	return path, true
}

func (p *proxy) forward(req *http.Request) (*http.Response, error) {
	req.RequestURI = ""
	req.URL.Scheme = "https"
	req.URL.Host = req.Host
	req.Header.Del("Accept-Encoding")
	upstream, err := p.upstream.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer upstream.Body.Close()
	body, err := io.ReadAll(upstream.Body)
	if err != nil {
		return nil, err
	}
	header := upstream.Header.Clone()
	for _, name := range []string{"Connection", "Content-Length", "Transfer-Encoding", "Content-Encoding"} {
		header.Del(name)
	}
	return response(req, upstream.StatusCode, header, body), nil
}

func (p *proxy) cached(req *http.Request, path string) (*http.Response, error) {
	meta, err := os.ReadFile(path + ".meta")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	header, err := textproto.NewReader(bufio.NewReader(bytes.NewReader(meta))).ReadMIMEHeader()
	if err != nil {
		return nil, fmt.Errorf("read %s.meta: %w", path, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return response(req, http.StatusOK, http.Header(header), body), nil
}

func (p *proxy) store(path string, resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	var meta bytes.Buffer
	if err := resp.Header.Write(&meta); err != nil {
		return err
	}
	meta.WriteString("\r\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := writeFile(path, body); err != nil {
		return err
	}
	return writeFile(path+".meta", meta.Bytes())
}

func (p *proxy) forget(path string) error {
	for _, name := range []string{path + ".meta", path} {
		if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func writeFile(name string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(name), filepath.Base(name)+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), name)
}

func response(req *http.Request, status int, header http.Header, body []byte) *http.Response {
	return &http.Response{
		StatusCode:    status,
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

func failed(req *http.Request, err error) *http.Response {
	slog.Error("devcache: request failed", "method", req.Method, "path", req.URL.Path, "error", err)
	return response(req, http.StatusBadGateway, http.Header{"Content-Type": {"text/plain"}}, []byte(err.Error()))
}
