package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/serve"
)

const (
	codeLength     = time.Minute
	tokenLength    = 30 * 24 * time.Hour
	maxRedirects   = 10
	maxClientName  = 200
	registerLimit  = 64 << 10
	resourcePath   = "/.well-known/oauth-protected-resource"
	authServerPath = "/.well-known/oauth-authorization-server"
)

type client struct {
	Name      string   `json:"n"`
	Redirects []string `json:"r"`
	Nonce     string   `json:"i"`
}

type grant struct {
	Client    string `json:"c"`
	Redirect  string `json:"r"`
	Challenge string `json:"p"`
	Email     string `json:"e"`
	Expires   int64  `json:"x"`
}

type bearer struct {
	Email  string `json:"e"`
	Issued int64  `json:"i"`
}

func origin(r *http.Request) string {
	return "https://" + r.Host
}

func mac(key []byte, purpose, body string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(purpose + "\n" + body))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func seal(key []byte, purpose string, v any) string {
	payload, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + mac(key, purpose, body)
}

func unseal(key []byte, purpose, sealed string, v any) bool {
	body, sig, ok := strings.Cut(sealed, ".")
	if !ok || !hmac.Equal([]byte(mac(key, purpose, body)), []byte(sig)) {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

func (s *Server) resource(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sdkauth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
			Resource:               origin(r) + path,
			AuthorizationServers:   []string{origin(r)},
			BearerMethodsSupported: []string{"header"},
			ResourceName:           "Helios",
		}).ServeHTTP(w, r)
	}
}

func (s *Server) authServer(w http.ResponseWriter, r *http.Request) {
	base := origin(r)
	serve.Write(w, r, http.StatusOK, map[string]any{
		"issuer":                                         base,
		"authorization_endpoint":                         base + "/oauth/authorize",
		"token_endpoint":                                 base + "/oauth/token",
		"registration_endpoint":                          base + "/oauth/register",
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"authorization_response_iss_parameter_supported": true,
	})
}

func oauthError(w http.ResponseWriter, r *http.Request, status int, code, description string) {
	w.Header().Set("Cache-Control", "no-store")
	serve.Write(w, r, status, map[string]string{"error": code, "error_description": description})
}

func redirectAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.Host == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var asked struct {
		Name      string   `json:"client_name"`
		Redirects []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, registerLimit)).Decode(&asked); err != nil {
		oauthError(w, r, http.StatusBadRequest, "invalid_client_metadata", "send the client's metadata as JSON")
		return
	}
	if len(asked.Redirects) == 0 || len(asked.Redirects) > maxRedirects {
		oauthError(w, r, http.StatusBadRequest, "invalid_redirect_uri", fmt.Sprintf("name between 1 and %d redirect_uris", maxRedirects))
		return
	}
	for _, redirect := range asked.Redirects {
		if !redirectAllowed(redirect) {
			oauthError(w, r, http.StatusBadRequest, "invalid_redirect_uri", "a redirect_uri is https, or http on a loopback address: "+redirect)
			return
		}
	}
	name := strings.TrimSpace(asked.Name)
	if name == "" {
		name = "An MCP client"
	}
	if len(name) > maxClientName {
		name = name[:maxClientName]
	}
	id := seal(s.deps.Key, "client", client{Name: name, Redirects: asked.Redirects, Nonce: rand.Text()})
	slog.InfoContext(r.Context(), "mcp: registered", "client", name, "redirects", asked.Redirects)
	w.Header().Set("Cache-Control", "no-store")
	serve.Write(w, r, http.StatusCreated, map[string]any{
		"client_id":                  id,
		"client_name":                name,
		"redirect_uris":              asked.Redirects,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
	})
}

type authorization struct {
	client   client
	clientID string
	redirect string
	state    string
	pkce     string
}

func (s *Server) authorization(query string) (authorization, error) {
	q, err := url.ParseQuery(strings.TrimPrefix(query, "?"))
	if err != nil {
		return authorization{}, access.Invalid("the request's address does not parse")
	}
	a := authorization{clientID: q.Get("client_id"), redirect: q.Get("redirect_uri"), state: q.Get("state"), pkce: q.Get("code_challenge")}
	if !unseal(s.deps.Key, "client", a.clientID, &a.client) {
		return authorization{}, access.Invalid("this client is not registered here; remove the connector and add it again")
	}
	if !slices.Contains(a.client.Redirects, a.redirect) {
		return authorization{}, access.Invalid("the redirect address is not one the client registered")
	}
	if q.Get("response_type") != "code" {
		return authorization{}, access.Invalid("only the code response type is supported")
	}
	if a.pkce == "" || q.Get("code_challenge_method") != "S256" {
		return authorization{}, access.Invalid("an S256 code challenge is required")
	}
	return a, nil
}

func (a authorization) back(params url.Values) string {
	if a.state != "" {
		params.Set("state", a.state)
	}
	sep := "?"
	if strings.Contains(a.redirect, "?") {
		sep = "&"
	}
	return a.redirect + sep + params.Encode()
}

type pending struct {
	Query string `json:"query"`
}

type described struct {
	Client   string `json:"client"`
	Redirect string `json:"redirect"`
	Email    string `json:"email"`
}

type onward struct {
	Redirect string `json:"redirect"`
}

func sameOrigin(r *http.Request) error {
	if r.Header.Get("Sec-Fetch-Site") != "same-origin" {
		return access.Forbidden("cross-site request refused")
	}
	return nil
}

func (s *Server) describe(r *http.Request, in pending) (described, error) {
	if err := sameOrigin(r); err != nil {
		return described{}, err
	}
	a, err := s.authorization(in.Query)
	if err != nil {
		return described{}, err
	}
	u, _ := url.Parse(a.redirect)
	return described{Client: a.client.Name, Redirect: u.Host, Email: auth.RealEmail(r)}, nil
}

func (s *Server) approve(r *http.Request, in pending) (onward, error) {
	if err := sameOrigin(r); err != nil {
		return onward{}, err
	}
	a, err := s.authorization(in.Query)
	if err != nil {
		return onward{}, err
	}
	email := auth.RealEmail(r)
	if s.deps.Data.Model().SignedIn(email) == "" {
		return onward{}, access.Forbidden("%s is not in the directory", email)
	}
	code := seal(s.deps.Key, "code", grant{Client: a.clientID, Redirect: a.redirect, Challenge: a.pkce, Email: email, Expires: s.deps.Now().Add(codeLength).Unix()})
	slog.InfoContext(r.Context(), "mcp: approved", "client", a.client.Name, "email", email)
	return onward{Redirect: a.back(url.Values{"code": {code}, "iss": {origin(r)}})}, nil
}

func (s *Server) deny(r *http.Request, in pending) (onward, error) {
	if err := sameOrigin(r); err != nil {
		return onward{}, err
	}
	a, err := s.authorization(in.Query)
	if err != nil {
		return onward{}, err
	}
	return onward{Redirect: a.back(url.Values{"error": {"access_denied"}, "iss": {origin(r)}})}, nil
}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, r, http.StatusBadRequest, "invalid_request", "send the token request as a form")
		return
	}
	if r.PostForm.Get("grant_type") != "authorization_code" {
		oauthError(w, r, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code is supported")
		return
	}
	var g grant
	if !unseal(s.deps.Key, "code", r.PostForm.Get("code"), &g) {
		oauthError(w, r, http.StatusBadRequest, "invalid_grant", "the code is not one this server issued")
		return
	}
	now := s.deps.Now()
	switch {
	case now.Unix() > g.Expires:
		oauthError(w, r, http.StatusBadRequest, "invalid_grant", "the code has expired")
		return
	case r.PostForm.Get("client_id") != g.Client:
		oauthError(w, r, http.StatusBadRequest, "invalid_grant", "the code was issued to another client")
		return
	case r.PostForm.Get("redirect_uri") != g.Redirect:
		oauthError(w, r, http.StatusBadRequest, "invalid_grant", "the redirect_uri differs from the authorization's")
		return
	case !hmac.Equal([]byte(challenge(r.PostForm.Get("code_verifier"))), []byte(g.Challenge)):
		oauthError(w, r, http.StatusBadRequest, "invalid_grant", "the code_verifier does not match the challenge")
		return
	}
	token := seal(s.deps.Key, "access", bearer{Email: g.Email, Issued: now.Unix()})
	slog.InfoContext(r.Context(), "mcp: token issued", "email", g.Email)
	w.Header().Set("Cache-Control", "no-store")
	serve.Write(w, r, http.StatusOK, map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(tokenLength.Seconds()),
	})
}

var errSignedOut = fmt.Errorf("%w: signed out, or no longer in the directory", sdkauth.ErrInvalidToken)

func (s *Server) verify(_ context.Context, token string, _ *http.Request) (*sdkauth.TokenInfo, error) {
	var a bearer
	if !unseal(s.deps.Key, "access", token, &a) {
		return nil, sdkauth.ErrInvalidToken
	}
	expires := time.Unix(a.Issued, 0).Add(tokenLength)
	if s.deps.Now().After(expires) {
		return nil, fmt.Errorf("%w: expired", sdkauth.ErrInvalidToken)
	}
	if out, ok := s.deps.Sessions.SignedOut(a.Email); ok && a.Issued <= out.Unix() {
		return nil, errSignedOut
	}
	if !s.deps.Member(a.Email) || s.deps.Data.Model().SignedIn(a.Email) == "" {
		return nil, errSignedOut
	}
	return &sdkauth.TokenInfo{UserID: a.Email, Expiration: expires}, nil
}
