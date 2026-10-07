package intercept

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"heliosian/internal/logging"
)

var (
	setup  sync.Once
	caKey  *ecdsa.PrivateKey
	caCert *x509.Certificate
	mu     sync.Mutex
	routes = map[string]*url.URL{}
)

func Install(host string, h http.Handler) {
	setup.Do(route)
	server, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{mintLeaf(host, caKey, caCert)}, NextProtos: []string{"http/1.1"}})
	if err != nil {
		logging.Fatal("intercept: listen", "host", host, "error", err)
	}
	go func() {
		logging.Fatal("intercept: serve", "host", host, "error", http.Serve(server, h))
	}()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		logging.Fatal("intercept: listen", "host", host, "error", err)
	}
	go func() {
		logging.Fatal("intercept: proxy", "host", host, "error", http.Serve(listener, tunnel{host: host, to: server.Addr().String()}))
	}()
	mu.Lock()
	routes[host] = &url.URL{Scheme: "http", Host: listener.Addr().String()}
	mu.Unlock()
	slog.Info("intercept: answering locally", "host", host)
}

func route() {
	caKey, caCert = mintCA()
	transport := http.DefaultTransport.(*http.Transport)
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	if transport.TLSClientConfig.RootCAs == nil {
		roots, err := x509.SystemCertPool()
		if err != nil {
			logging.Fatal("intercept: system roots", "error", err)
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	transport.TLSClientConfig.RootCAs.AddCert(caCert)
	next := transport.Proxy
	transport.Proxy = func(r *http.Request) (*url.URL, error) {
		mu.Lock()
		through, ok := routes[r.URL.Hostname()]
		mu.Unlock()
		if ok {
			return through, nil
		}
		return next(r)
	}
}

type tunnel struct {
	host, to string
}

func (t tunnel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect || r.Host != t.host+":443" {
		http.Error(w, "intercept tunnels only "+t.host, http.StatusBadGateway)
		return
	}
	upstream, err := net.Dial("tcp", t.to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer upstream.Close()
	conn, _, err := http.NewResponseController(w).Hijack()
	if err != nil {
		slog.Error("intercept: hijack", "error", err)
		return
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		io.Copy(upstream, conn)
		close(done)
	}()
	io.Copy(conn, upstream)
	conn.Close()
	<-done
}

func mintCA() (*ecdsa.PrivateKey, *x509.Certificate) {
	key := newKey()
	template := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "heliosian intercept"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(0, 0, 30),
		KeyUsage:              x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		logging.Fatal("intercept: sign ca", "error", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		logging.Fatal("intercept: parse ca", "error", err)
	}
	return key, cert
}

func mintLeaf(host string, caKey *ecdsa.PrivateKey, ca *x509.Certificate) tls.Certificate {
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
		logging.Fatal("intercept: sign leaf", "error", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func newKey() *ecdsa.PrivateKey {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		logging.Fatal("intercept: generate key", "error", err)
	}
	return key
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		logging.Fatal("intercept: generate serial", "error", err)
	}
	return n
}
