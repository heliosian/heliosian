package mcp

import (
	"net/http"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/db"
	"heliosian/internal/model"
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
	Domain   string
}

type Server struct {
	deps    Deps
	server  *sdk.Server
	handler http.Handler
	origin  func(app string) string
}

const about = "Helios School, a small K-8 school on the San Francisco peninsula coast, and its parents' association, the HCA: the community's people, families, classrooms and teachers, calendar and events, volunteer activities, fundraiser parties, email lists, and the newsletters and school mail families received."

const instructions = `This server is ` + about + ` Use it for any question about Helios, the school, a family, child, teacher or classroom there, what is coming up, or what the school has sent out. It reads as the person who connected, checked against the same policies the apps use, so it answers exactly what their own pages would show them, nothing more.

A record with a page on the Helios apps carries href, the address of that page: a person, family, classroom or grade on Helios Who?, an event on Helios When, a volunteer activity on HCA-Team, a party on Helios Celebrate, an email list on Helios Loop, a Helios Wiki page, or a document's own address on the web. Whenever you name such a record, link it to its href. Never write an address for a record that has none, and never build one from an ID.

Start with helios_search or helios_whoami. For anything structured, helios_describe_schema and helios_describe_table give the tables, helios_policies the definitions a query may call, and helios_query runs the query language below. helios_get, helios_group and helios_read_document look one row up in depth; helios_find_people and helios_events are shortcuts for common questions.

` + db.Language

func Register(mux *http.ServeMux, deps Deps) {
	s := &Server{deps: deps, origin: model.Origin(deps.Domain)}
	s.server = sdk.NewServer(&sdk.Implementation{Name: "helios-school", Title: "Helios School", Description: about, Version: "1"}, &sdk.ServerOptions{Instructions: instructions})
	s.tools()
	s.handler = sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return s.server }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		serve.File(w, r, "web/mcp/index.html")
	})
	mux.HandleFunc("GET /oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		serve.File(w, r, "web/mcp/authorize.html")
	})
	mux.HandleFunc("GET "+resourcePath, s.resource("/"))
	mux.HandleFunc("GET "+resourcePath+"/mcp", s.resource("/mcp"))
	mux.HandleFunc("GET "+authServerPath, s.authServer)
	mux.HandleFunc("POST /oauth/register", s.register)
	mux.HandleFunc("POST /oauth/token", s.token)
	mux.HandleFunc("POST /api/mcp/request", serve.JSON(s.describe))
	mux.HandleFunc("POST /api/mcp/approve", serve.JSON(s.approve))
	mux.HandleFunc("POST /api/mcp/deny", serve.JSON(s.deny))
	mux.HandleFunc("POST /{$}", s.guarded(""))
	mux.HandleFunc("/mcp", s.guarded("/mcp"))
}

func (s *Server) guarded(suffix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guard := sdkauth.RequireBearerToken(s.verify, &sdkauth.RequireBearerTokenOptions{ResourceMetadataURL: origin(r) + resourcePath + suffix})
		guard(s.handler).ServeHTTP(w, r)
	}
}
