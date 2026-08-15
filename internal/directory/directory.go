// Package directory serves the school directory app.
package directory

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
)

type app struct {
	cache *Cache
}

func Register(mux *http.ServeMux, cache *Cache) {
	a := app{cache: cache}
	mux.HandleFunc("GET /directory/{$}", a.index)
	mux.HandleFunc("GET /directory/api/model", a.model)
	mux.Handle("GET /directory/static/", http.StripPrefix("/directory/static/", http.FileServer(http.Dir("web/directory/static"))))
}

func (a app) index(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("web/directory/index.html")
	if err != nil {
		serverError(w, err)
		return
	}
	if err := t.Execute(w, nil); err != nil {
		log.Printf("[ERROR] render directory index: %v", err)
	}
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.cache.Model()); err != nil {
		log.Printf("[ERROR] encode model: %v", err)
	}
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("[ERROR] %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
