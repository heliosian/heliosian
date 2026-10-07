package mcp

import (
	"net/http"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/db"
	"heliosian/internal/serve"
)

type Deps struct {
	Data     *db.Store
	Search   *db.Searcher
	Bucket   *blob.Bucket
	Key      []byte
	Sessions auth.Sessions
	Member   func(email string) bool
	Now      func() time.Time
}

type Server struct {
	deps    Deps
	server  *sdk.Server
	handler http.Handler
}

const instructions = `Helios's data model - its people, families, classrooms, groups, events, volunteer activities, parties, email lists and the documents and mail the community received - read as the person who connected. Every read is checked against the same policies the apps use, so it answers exactly what their own pages would show them, nothing more.

Start with describe_schema, then describe_table for the tables you need, and policies for the definitions a query may call. query runs the query language below. search finds people, groups and documents by words and by meaning. whoami is the person connected. get, group, read_document and history look one row up in depth; find_people and events are shortcuts for common questions.

` + db.Language

func Register(mux *http.ServeMux, deps Deps) {
	s := &Server{deps: deps}
	s.server = sdk.NewServer(&sdk.Implementation{Name: "helios", Title: "Helios", Version: "1"}, &sdk.ServerOptions{Instructions: instructions})
	s.tools()
	s.handler = sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return s.server }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		serve.File(w, r, "web/mcp/index.html")
	})
	mux.HandleFunc("GET /oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		serve.File(w, r, "web/mcp/authorize.html")
	})
	mux.HandleFunc("GET "+resourcePath, s.resource)
	mux.HandleFunc("GET "+resourcePath+"/mcp", s.resource)
	mux.HandleFunc("GET "+authServerPath, s.authServer)
	mux.HandleFunc("POST /oauth/register", s.register)
	mux.HandleFunc("POST /oauth/token", s.token)
	mux.HandleFunc("POST /api/mcp/request", serve.JSON(s.describe))
	mux.HandleFunc("POST /api/mcp/approve", serve.JSON(s.approve))
	mux.HandleFunc("POST /api/mcp/deny", serve.JSON(s.deny))
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		guard := sdkauth.RequireBearerToken(s.verify, &sdkauth.RequireBearerTokenOptions{ResourceMetadataURL: origin(r) + resourcePath + "/mcp"})
		guard(s.handler).ServeHTTP(w, r)
	})
}
