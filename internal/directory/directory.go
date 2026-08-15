// Package directory serves the school directory app.
package directory

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"

	"heliosian/internal/data"
)

type app struct {
	source data.Source
}

func Register(mux *http.ServeMux, source data.Source) {
	a := app{source: source}
	mux.HandleFunc("GET /directory/{$}", a.index)
	mux.HandleFunc("GET /directory/api/people", a.people)
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

type person struct {
	ID            string `json:"id"`
	FirstName     string `json:"firstName"`
	LastName      string `json:"lastName"`
	Role          string `json:"role"`
	Grade         string `json:"grade"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	Pronunciation string `json:"pronunciation"`
	FamilyName    string `json:"familyName"`
	Address       string `json:"address"`
	FamilyPhone   string `json:"familyPhone"`
}

func (a app) people(w http.ResponseWriter, r *http.Request) {
	families, err := a.source.Table("directory", "families")
	if err != nil {
		serverError(w, err)
		return
	}
	familiesByID := map[string]map[string]string{}
	for _, family := range families {
		familiesByID[family["id"]] = family
	}
	rows, err := a.source.Table("directory", "people")
	if err != nil {
		serverError(w, err)
		return
	}
	people := []person{}
	for _, row := range rows {
		family, ok := familiesByID[row["family_id"]]
		if !ok {
			serverError(w, fmt.Errorf("person %s has unknown family_id %q", row["id"], row["family_id"]))
			return
		}
		people = append(people, person{
			ID:            row["id"],
			FirstName:     row["first_name"],
			LastName:      row["last_name"],
			Role:          row["role"],
			Grade:         row["grade"],
			Email:         row["email"],
			Phone:         row["phone"],
			Pronunciation: row["pronunciation"],
			FamilyName:    family["name"],
			Address:       family["address"],
			FamilyPhone:   family["phone"],
		})
	}
	sort.Slice(people, func(i, j int) bool {
		if people[i].LastName != people[j].LastName {
			return people[i].LastName < people[j].LastName
		}
		return people[i].FirstName < people[j].FirstName
	})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(people); err != nil {
		log.Printf("[ERROR] encode people: %v", err)
	}
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("[ERROR] %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
