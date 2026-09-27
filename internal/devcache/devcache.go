package devcache

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"heliosian/internal/logging"
)

const (
	dir        = "local/cache/blobs"
	host       = "storage.googleapis.com"
	objectPath = "/storage/v1/b/"
)

func Install() {
	caKey, caCert := mintCA()
	leaf := mintLeaf(caKey, caCert)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		logging.Fatal("devcache: listen", "error", err)
	}
	transport := http.DefaultTransport.(*http.Transport)
	p := &proxy{
		dir:      dir,
		upstream: transport.Clone(),
		tls:      &tls.Config{Certificates: []tls.Certificate{leaf}, NextProtos: []string{"http/1.1"}},
	}
	go func() {
		logging.Fatal("devcache: serve", "error", http.Serve(listener, p))
	}()
	roots, err := x509.SystemCertPool()
	if err != nil {
		logging.Fatal("devcache: system roots", "error", err)
	}
	roots.AddCert(caCert)
	through := &url.URL{Scheme: "http", Host: listener.Addr().String()}
	transport.Proxy = func(r *http.Request) (*url.URL, error) {
		if r.URL.Hostname() == host {
			return through, nil
		}
		return http.ProxyFromEnvironment(r)
	}
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	slog.Info("devcache: caching storage reads", "dir", dir)
}

func mintCA() (*ecdsa.PrivateKey, *x509.Certificate) {
	key := newKey()
	template := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "heliosian devcache"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(0, 0, 30),
		KeyUsage:              x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		logging.Fatal("devcache: sign ca", "error", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		logging.Fatal("devcache: parse ca", "error", err)
	}
	return key, cert
}

func mintLeaf(caKey *ecdsa.PrivateKey, ca *x509.Certificate) tls.Certificate {
	key := newKey()
	template := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(0, 0, 30),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		logging.Fatal("devcache: sign leaf", "error", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func newKey() *ecdsa.PrivateKey {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		logging.Fatal("devcache: generate key", "error", err)
	}
	return key
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		logging.Fatal("devcache: generate serial", "error", err)
	}
	return n
}

type proxy struct {
	dir      string
	upstream http.RoundTripper
	tls      *tls.Config
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect || r.Host != host+":443" {
		http.Error(w, "devcache tunnels only "+host, http.StatusBadGateway)
		return
	}
	conn, _, err := http.NewResponseController(w).Hijack()
	if err != nil {
		slog.Error("devcache: hijack", "error", err)
		return
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	tlsConn := tls.Server(conn, p.tls)
	reader := bufio.NewReader(tlsConn)
	for {
		req, err := http.ReadRequest(reader)
		if err != nil {
			return
		}
		resp := p.answer(req)
		if err := resp.Write(tlsConn); err != nil || req.Close || resp.Close {
			return
		}
	}
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
	resp := response(req, http.StatusBadGateway, http.Header{"Content-Type": {"text/plain"}}, []byte(err.Error()))
	resp.Close = true
	return resp
}
