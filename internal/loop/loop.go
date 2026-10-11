package loop

import (
	"net/http"

	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

func Register(mux *http.ServeMux, about *sharecard.About) {
	mux.Handle("GET /open/share/about.png", about)
	for _, path := range []string{"/{$}", "/new", "/groups/{key}"} {
		mux.HandleFunc("GET "+path, page)
	}
}

func page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, "web/loop/index.html")
}
