package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"heliosian/internal/artifacts"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/intercept"
	"heliosian/internal/model"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

const (
	rowan    = "rowan@example.com"
	rowanID  = "per00000000002"
	mayaID   = "per00000000003"
	picnic   = "grp00000000040"
	camping  = "doc00000000106"
	redirect = "http://127.0.0.1:33418/callback"
	verifier = "a-verifier-long-enough-to-be-a-real-one-0123456789"
)

type sessions struct {
	mu  sync.Mutex
	out map[string]time.Time
}

func (s *sessions) SignedOut(email string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.out[email]
	return t, ok
}

func (s *sessions) SignOut(_ context.Context, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.out[email] = time.Now()
	return nil
}

type fixture struct {
	server   *httptest.Server
	sessions *sessions
	search   *db.Searcher
}

func setup(t *testing.T) fixture {
	t.Helper()
	intercept.GoogleLogin(t.TempDir())
	intercept.Install(intercept.VertexHost, intercept.Vertex())
	intercept.Install(intercept.ClaudeHost, intercept.Claude())
	vertex, err := artifacts.NewVertex()
	if err != nil {
		t.Fatal(err)
	}
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	s, err := db.NewStore(dir, dir, queue, db.NewSearchIndex())
	if err != nil {
		t.Fatal(err)
	}
	bucket := blob.NewMemoryBucket()
	for _, content := range []string{"22792c45ef58dad79968050aa89fcc2eecb46446f8fd7d053e65bbef5525b392", "20459606b3b5dc2fa29911a43070c5de1d69f7fbeb426c9b5a9f4b5d7d55c285"} {
		body, err := os.ReadFile("../../sampledata/bucket/content/" + content)
		if err != nil {
			t.Fatal(err)
		}
		if err := bucket.Put(context.Background(), "content/"+content, "text/markdown", body); err != nil {
			t.Fatal(err)
		}
	}
	signed := &sessions{out: map[string]time.Time{}}
	search := db.NewSearcher(s, queue, bucket, vertex, model.Origin("heliosian.com"))
	mux := http.NewServeMux()
	Register(mux, Deps{
		Data:     s,
		Search:   search,
		Bucket:   bucket,
		Key:      []byte("test"),
		Sessions: signed,
		Member:   func(string) bool { return true },
		Now:      time.Now,
		Domain:   "heliosian.com",
	})
	server := httptest.NewServer(auth.Fixed(rowan, mux))
	t.Cleanup(server.Close)
	return fixture{server: server, sessions: signed, search: search}
}

func (f fixture) post(t *testing.T, path, kind, body string, headers map[string]string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, f.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", kind)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func challengeOf(v string) string {
	sum := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (f fixture) register(t *testing.T) string {
	t.Helper()
	status, out := f.post(t, "/oauth/register", "application/json", `{"client_name": "Test Client", "redirect_uris": ["`+redirect+`"]}`, nil)
	if status != http.StatusCreated {
		t.Fatalf("register answered %d: %v", status, out)
	}
	return out["client_id"].(string)
}

func (f fixture) authorizeQuery(client, challenge string) string {
	return "?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {client},
		"redirect_uri":          {redirect},
		"state":                 {"xyz"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()
}

func (f fixture) approve(t *testing.T, client string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"query": f.authorizeQuery(client, challengeOf(verifier))})
	status, out := f.post(t, "/api/mcp/approve", "application/json", string(body), map[string]string{"Sec-Fetch-Site": "same-origin"})
	if status != http.StatusOK {
		t.Fatalf("approve answered %d: %v", status, out)
	}
	back, err := url.Parse(out["redirect"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if got := back.Scheme + "://" + back.Host + back.Path; got != redirect {
		t.Fatalf("approve sends the browser to %s", got)
	}
	if back.Query().Get("state") != "xyz" {
		t.Fatalf("approve drops the state: %s", back)
	}
	return back.Query().Get("code")
}

func (f fixture) exchange(t *testing.T, client, code, verifier string) (int, map[string]any) {
	t.Helper()
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {client}, "redirect_uri": {redirect}, "code_verifier": {verifier}}
	return f.post(t, "/oauth/token", "application/x-www-form-urlencoded", form.Encode(), nil)
}

func (f fixture) token(t *testing.T) string {
	t.Helper()
	client := f.register(t)
	status, out := f.exchange(t, client, f.approve(t, client), verifier)
	if status != http.StatusOK {
		t.Fatalf("token answered %d: %v", status, out)
	}
	return out["access_token"].(string)
}

type bearerTransport string

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+string(b))
	return http.DefaultTransport.RoundTrip(r)
}

func (f fixture) connect(t *testing.T, token string) *sdk.ClientSession {
	t.Helper()
	return f.connectAt(t, token, "/mcp")
}

func (f fixture) connectAt(t *testing.T, token, endpoint string) *sdk.ClientSession {
	t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &sdk.StreamableClientTransport{
		Endpoint:   f.server.URL + endpoint,
		HTTPClient: &http.Client{Transport: bearerTransport(token)},
		MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func callTool(t *testing.T, session *sdk.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	text := ""
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			text += tc.Text
		}
	}
	return text, res.IsError
}

func TestConnectingReadsAsThePersonWhoApproved(t *testing.T) {
	f := setup(t)
	session := f.connect(t, f.token(t))
	text, failed := callTool(t, session, "helios_whoami", nil)
	if failed {
		t.Fatalf("whoami failed: %s", text)
	}
	var who struct {
		Person struct {
			Rows []map[string]string `json:"rows"`
		} `json:"person"`
	}
	if err := json.Unmarshal([]byte(text), &who); err != nil {
		t.Fatal(err)
	}
	if len(who.Person.Rows) != 1 || who.Person.Rows[0]["id"] != rowanID {
		t.Fatalf("whoami answered %s", text)
	}
}

func TestTheRootServesMCPToo(t *testing.T) {
	f := setup(t)
	text, failed := callTool(t, f.connectAt(t, f.token(t), "/"), "helios_whoami", nil)
	if failed || !strings.Contains(text, rowanID) {
		t.Fatalf("whoami at the root: %s", text)
	}
	res, err := http.Get(f.server.URL + "/.well-known/oauth-protected-resource")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	resource := map[string]any{}
	json.NewDecoder(res.Body).Decode(&resource)
	if got, _ := resource["resource"].(string); !strings.HasSuffix(got, "/") || strings.HasSuffix(got, "/mcp") {
		t.Errorf("the root's resource is %q", got)
	}
}

func TestAWellKnownDocumentNotServedIsMissing(t *testing.T) {
	f := setup(t)
	res, err := http.Get(f.server.URL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("openid-configuration answered %d", res.StatusCode)
	}
}

func TestAMissingOrForeignTokenPointsAtTheMetadata(t *testing.T) {
	f := setup(t)
	for _, endpoint := range []string{"/mcp", "/"} {
		for name, token := range map[string]string{"none": "", "forged": "e30.bm90LWEtc2lnbmF0dXJl"} {
			req, _ := http.NewRequest(http.MethodPost, f.server.URL+endpoint, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s %s: answered %d", endpoint, name, res.StatusCode)
			}
			if got := res.Header.Get("WWW-Authenticate"); !strings.Contains(got, "resource_metadata=") || !strings.HasSuffix(got, strings.TrimSuffix(resourcePath+endpoint, "/")+`"`) {
				t.Errorf("%s %s: WWW-Authenticate is %q", endpoint, name, got)
			}
		}
	}
}

func TestTheMetadataNamesTheEndpoints(t *testing.T) {
	f := setup(t)
	res, err := http.Get(f.server.URL + "/.well-known/oauth-authorization-server")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	meta := map[string]any{}
	json.NewDecoder(res.Body).Decode(&meta)
	for key, want := range map[string]string{"authorization_endpoint": "/oauth/authorize", "token_endpoint": "/oauth/token", "registration_endpoint": "/oauth/register"} {
		if got, _ := meta[key].(string); !strings.HasSuffix(got, want) {
			t.Errorf("%s is %q", key, got)
		}
	}
	res, err = http.Get(f.server.URL + "/.well-known/oauth-protected-resource/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	resource := map[string]any{}
	json.NewDecoder(res.Body).Decode(&resource)
	if got, _ := resource["resource"].(string); !strings.HasSuffix(got, "/mcp") {
		t.Errorf("the resource is %q", got)
	}
}

func TestTheCodeIsRefusedWithoutItsVerifier(t *testing.T) {
	f := setup(t)
	client := f.register(t)
	code := f.approve(t, client)
	if status, out := f.exchange(t, client, code, "the-wrong-verifier-the-wrong-verifier-0123456789"); status != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("a wrong verifier got %d: %v", status, out)
	}
	if status, out := f.exchange(t, f.register(t), code, verifier); status != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("another client's exchange got %d: %v", status, out)
	}
}

func TestOnlyRegisteredRedirectsAndSameOriginApprovals(t *testing.T) {
	f := setup(t)
	if status, _ := f.post(t, "/oauth/register", "application/json", `{"redirect_uris": ["http://evil.example.com/cb"]}`, nil); status != http.StatusBadRequest {
		t.Errorf("a plain-http redirect elsewhere registered: %d", status)
	}
	client := f.register(t)
	query := strings.Replace(f.authorizeQuery(client, challengeOf(verifier)), url.QueryEscape(redirect), url.QueryEscape("https://evil.example.com/cb"), 1)
	body, _ := json.Marshal(map[string]string{"query": query})
	if status, _ := f.post(t, "/api/mcp/approve", "application/json", string(body), map[string]string{"Sec-Fetch-Site": "same-origin"}); status != http.StatusBadRequest {
		t.Errorf("an unregistered redirect was approved: %d", status)
	}
	body, _ = json.Marshal(map[string]string{"query": f.authorizeQuery(client, challengeOf(verifier))})
	if status, _ := f.post(t, "/api/mcp/approve", "application/json", string(body), map[string]string{"Sec-Fetch-Site": "cross-site"}); status != http.StatusForbidden {
		t.Errorf("a cross-site approval answered %d", status)
	}
}

func TestSigningOutRevokesTheToken(t *testing.T) {
	f := setup(t)
	token := f.token(t)
	f.sessions.SignOut(context.Background(), rowan)
	req, _ := http.NewRequest(http.MethodPost, f.server.URL+"/mcp", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a token from before sign-out answered %d", res.StatusCode)
	}
}

func TestTools(t *testing.T) {
	f := setup(t)
	session := f.connect(t, f.token(t))
	for _, c := range []struct {
		tool  string
		args  map[string]any
		wants []string
		fails bool
	}{
		{"helios_describe_schema", nil, []string{"PERSON:", "DOCUMENT:", "EFFECTIVE_MEMBER:"}, false},
		{"helios_describe_table", map[string]any{"table": "person"}, []string{"- name_show (text)", "Pointed at by:", "PERSON_EMAIL.person"}, false},
		{"helios_policies", nil, []string{"(define (visible @g)"}, false},
		{"helios_query", map[string]any{"query": `(from PERSON (where (= id "` + rowanID + `")))`}, []string{`"count":1`, `"href":"https://who.heliosian.com/people/` + rowanID + `"`}, false},
		{"helios_query", map[string]any{"query": "(from PERSON (where (= grde \"3\")))"}, []string{"did you mean grade"}, true},
		{"helios_get", map[string]any{"id": picnic}, []string{`"table":"GROUP"`, `"MEMBER.group"`, `"href":"https://when.heliosian.com/e/`}, false},
		{"helios_get", map[string]any{"id": "not-an-id"}, []string{"is not an ID"}, true},
		{"helios_group", map[string]any{"id": picnic}, []string{`"members"`, `"memberCount"`, `"href":"https://who.heliosian.com/people/`}, false},
		{"helios_group", map[string]any{"id": rowanID}, []string{"not a GROUP"}, true},
		{"helios_read_document", map[string]any{"id": camping}, []string{"# Camping Trips", "https://wiki.heliosian.com/p/Activities/Camping-Trips", "doc00000000109 side", "## wiki (" + camping + ")"}, false},
		{"helios_similar", map[string]any{"id": camping}, []string{"no search entry"}, true},
		{"helios_find_people", map[string]any{"role": "parent"}, []string{rowanID}, false},
		{"helios_find_people", nil, []string{"name at least one"}, true},
		{"helios_events", map[string]any{"from": "2026-01-01", "to": "2026-12-31"}, []string{`"count"`}, false},
		{"helios_events", map[string]any{"from": "tomorrow"}, []string{"YYYY-MM-DD"}, true},
	} {
		text, failed := callTool(t, session, c.tool, c.args)
		if failed != c.fails {
			t.Errorf("%s %v: failed %v: %s", c.tool, c.args, failed, text)
			continue
		}
		for _, want := range c.wants {
			if !strings.Contains(text, want) {
				t.Errorf("%s %v: no %q in %.600s", c.tool, c.args, want, text)
			}
		}
	}
}

func TestSearchNamesEachHit(t *testing.T) {
	f := setup(t)
	intercept.Install(intercept.ClaudeHost, testkit.SearchClaude())
	f.search.StartMaking("test")
	session := f.connect(t, f.token(t))
	text := ""
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if _, err := f.search.Make(rowanID); err != nil {
			continue
		}
		if _, err := f.search.Make(mayaID); err != nil {
			continue
		}
		text, _ = callTool(t, session, "helios_search", map[string]any{"words": "Rowan Ashdown"})
		if strings.Contains(text, `"meaning":[{`) {
			break
		}
	}
	var found map[string]db.SearchResults
	if err := json.Unmarshal([]byte(text), &found); err != nil {
		t.Fatalf("%v: %s", err, text)
	}
	people := found["PERSON"]
	if len(people.Words) == 0 || people.Words[0].ID != rowanID || people.Words[0].Name == "" || people.Words[0].Href != "https://who.heliosian.com/people/"+rowanID {
		t.Fatalf("the word search answered %s", text)
	}
	if len(people.Meaning) == 0 {
		t.Fatalf("the meaning search found nothing: %s", text)
	}
	for _, r := range people.Meaning {
		if r.ID == rowanID {
			t.Fatalf("Rowan, a name alone, was found by meaning: %s", text)
		}
	}
}

func TestEveryToolSaysItIsHelios(t *testing.T) {
	f := setup(t)
	listed, err := f.connect(t, f.token(t)).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) == 0 {
		t.Fatal("no tools listed")
	}
	for _, tool := range listed.Tools {
		if !strings.HasPrefix(tool.Name, "helios_") || !strings.Contains(tool.Description, "Helios") {
			t.Errorf("%s does not say it is Helios's: %s", tool.Name, tool.Description)
		}
	}
}

func TestDescribeTableLeavesOutPrivateColumns(t *testing.T) {
	f := setup(t)
	text, _ := callTool(t, f.connect(t, f.token(t)), "helios_describe_table", map[string]any{"table": "PERSON"})
	if strings.Contains(text, "- vc_phone (") || strings.Contains(text, "- consent (") {
		t.Fatalf("a private column is described: %s", text)
	}
}
