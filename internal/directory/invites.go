// Invites reads the Invite List Builder sheet: a manifest tab (_Services) naming
// one tab per destination system (Greenvelope, Evite, ...), each holding that
// system's own column header plus a template row of {{ parameter }} tokens the
// client fills in per family. Keeping the templates in the sheet - not in code -
// means adding a new system, or fixing a system's own header quirk, is a sheet
// edit, not a deploy.
package directory

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"heliosian/internal/data"
)

// InviteTemplate is one row of _Services plus the header and template row(s) read
// out of the tab it points to, exactly as that tab holds them - including any
// quirk (a duplicate column name, a blank cell) the destination system itself
// requires, since these are meant to be filled in and re-uploaded unchanged.
//
// Description and Notes serve different readers: Description is the one-line,
// no-jargon summary (does this group families into one invite or list everyone
// separately, does a kid get emailed) the Invites page shows next to the
// system's name so a non-technical person can pick the right one. Notes is the
// deeper documentation for whoever edits this sheet later - source links,
// column quirks, why a template is shaped the way it is - and isn't rendered in
// the app.
type InviteTemplate struct {
	Name        string `json:"name"`
	Sheet       string `json:"sheet"`
	Description string `json:"description,omitempty"`
	Notes       string `json:"notes,omitempty"`
	HeaderRow   bool   `json:"headerRow"`
	// SupportsGroups is sourced from the destination system's own docs, not
	// inferred from the template - whether it has any real way to address a
	// couple/family/group as one invitee (a dedicated field, or a documented
	// convention for cramming several names into one) versus being built for
	// one individual contact per row. The client uses it both to decide which
	// greeting styles to offer and whether the Group by family switch (one row
	// per family vs. one row per person) is offered at all - a system that
	// can't really address a group doesn't get presented as one, even if its
	// docs describe a workaround for cramming names into one field.
	SupportsGroups bool       `json:"supportsGroups"`
	Header         []string   `json:"header"`
	Rows           [][]string `json:"rows"`
}

// GreetingTemplate is one row of _Greetings: a named greeting style (shown to
// the person exporting, in the picker embedded in the CSV preview's greeting
// column) plus its Format - not a {{ token }} template but a worked example
// written against one made-up family (parents Pat and Quinn, kids Ali and Bo,
// surname Ender), so editing or adding a greeting means writing out what it
// would look like for that family rather than learning template syntax. The
// client recognizes fixed phrases from that example ("Ali & Bo Ender" for the
// joined kids, "Pat & Quinn Ender" for the joined adults, and so on) and
// substitutes the real family's names wherever they appear - see
// buildGreeting in app.js, the one place that mapping is defined.
type GreetingTemplate struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	// Grouped and Individual are independent, not two ends of one flag - a
	// greeting that doesn't reference the family/kid example phrases at all
	// (a fixed line with no names in it) can genuinely apply to both a whole
	// family's invite and a single person's, so a row can check either box,
	// both, or (someone forgot to fill it in) default to both.
	Grouped    bool `json:"grouped"`
	Individual bool `json:"individual"`
	// CreatedBy is who added this greeting - recorded for anyone editing the
	// sheet later to see at a glance, not currently shown in the app itself.
	// Blank for one of the built-in rows nobody "added" through the app.
	CreatedBy string `json:"createdBy,omitempty"`
}

const invitesApp = "invites"

func loadGreetingTemplates(source data.Source) ([]GreetingTemplate, error) {
	_, rows, err := source.Table(invitesApp, "_Greetings")
	if err != nil {
		return nil, err
	}
	greetings := make([]GreetingTemplate, 0, len(rows))
	for _, row := range rows {
		name, format := row["Name"], row["Format"]
		if name == "" || format == "" {
			continue
		}
		greetings = append(greetings, GreetingTemplate{
			Name:       name,
			Format:     format,
			Grouped:    row["Grouped"] != "No",
			Individual: row["Individual"] != "No",
			CreatedBy:  row["Email"],
		})
	}
	return greetings, nil
}

func loadInviteTemplates(source data.Source) ([]InviteTemplate, error) {
	_, systems, err := source.Table(invitesApp, "_Services")
	if err != nil {
		return nil, err
	}
	templates := make([]InviteTemplate, 0, len(systems))
	for _, sys := range systems {
		name, tab := sys["Display Name"], sys["Sheet"]
		if name == "" || tab == "" {
			continue
		}
		raw, err := source.Raw(invitesApp, tab)
		if err != nil {
			return nil, fmt.Errorf("load %s template (tab %q): %w", name, tab, err)
		}
		if len(raw) == 0 {
			continue
		}
		templates = append(templates, InviteTemplate{
			Name:        name,
			Sheet:       tab,
			Description: sys["Description"],
			Notes:       sys["Notes"],
			// A blank/missing cell defaults each of these to "yes" - the common
			// case for both columns (every system but Evite wants its header
			// included; most systems support naming a group) - so a manifest row
			// added before a column existed still behaves sensibly.
			HeaderRow:      sys["Header Row"] != "No",
			SupportsGroups: sys["Supports Groups"] != "No",
			Header:         raw[0],
			Rows:           raw[1:],
		})
	}
	return templates, nil
}

// RegisterInvites serves the parsed Invite List Builder templates to the
// client, which runs the actual per-family substitution (it already has the
// filtered, formatted family data the templates draw on), and lets anyone add
// their own greeting to _Greetings. Source/writer are the same ones
// directory/preferences read and write through; if no "invites" spreadsheet is
// configured, GET comes back with systems/greetings empty and an explanatory
// error string rather than a failed request, since this is one optional
// feature, not the app - POST simply fails the same way any write would with
// no spreadsheet behind it.
func RegisterInvites(mux *http.ServeMux, cache *Cache, source data.Source, writer data.Writer) {
	mux.HandleFunc("GET /api/directory/invite-templates", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		view := struct {
			Systems   []InviteTemplate   `json:"systems"`
			Greetings []GreetingTemplate `json:"greetings"`
			Error     string             `json:"error,omitempty"`
		}{}
		templates, err := loadInviteTemplates(source)
		if err != nil {
			log.Printf("[ERROR] load invite templates: %v", err)
			view.Error = err.Error()
		} else {
			view.Systems = templates
		}
		greetings, err := loadGreetingTemplates(source)
		if err != nil {
			log.Printf("[ERROR] load greeting templates: %v", err)
			if view.Error == "" {
				view.Error = err.Error()
			}
		} else {
			view.Greetings = greetings
		}
		if err := json.NewEncoder(w).Encode(view); err != nil {
			log.Printf("[ERROR] encode invite templates: %v", err)
		}
	})

	// original, when present, names an existing _Greetings row to update in
	// place (Upsert keyed on Name) instead of appending a new one - how the
	// client asks to edit rather than create. Only the row's own creator may
	// do that; anyone editing someone else's (or a built-in row's, which has
	// no CreatedBy to match) is rejected rather than silently taking it over.
	mux.HandleFunc("POST /api/directory/greetings", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		format := strings.TrimSpace(r.FormValue("format"))
		original := strings.TrimSpace(r.FormValue("original"))
		if format == "" {
			http.Error(w, "format is required", http.StatusBadRequest)
			return
		}
		if len(format) > 200 {
			http.Error(w, "format is too long", http.StatusBadRequest)
			return
		}
		// No separate name to author - the greeting's Format text doubles as
		// its Name (what the picker shows), same as every built-in row where
		// the two happen to read almost the same. Editable later via a
		// sheet-side rename if it ever needs to differ.
		name := format
		email := effectiveEmail(cache, r)
		cells := map[string]string{
			"Name":       name,
			"Format":     format,
			"Grouped":    yesNo(r.FormValue("grouped") == "1"),
			"Individual": yesNo(r.FormValue("individual") == "1"),
			"Email":      email,
		}
		if original != "" {
			owned, err := ownsGreeting(source, original, email)
			if err != nil {
				log.Printf("[ERROR] load greetings for edit: %v", err)
				http.Error(w, "failed to save greeting", http.StatusInternalServerError)
				return
			}
			if !owned {
				http.Error(w, "you can only edit greetings you created", http.StatusForbidden)
				return
			}
			if err := writer.Upsert(invitesApp, "_Greetings", "Name", original, cells); err != nil {
				log.Printf("[ERROR] update greeting %q: %v", name, err)
				http.Error(w, "failed to save greeting", http.StatusInternalServerError)
				return
			}
			log.Printf("greeting: %s edited %q -> %q", email, original, name)
		} else {
			row := []string{name, format, cells["Grouped"], cells["Individual"], email}
			if err := writer.Append(invitesApp, "_Greetings", row); err != nil {
				log.Printf("[ERROR] save greeting %q: %v", name, err)
				http.Error(w, "failed to save greeting", http.StatusInternalServerError)
				return
			}
			log.Printf("greeting: %s added %q", email, name)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Same ownership rule as editing - only the greeting's own creator can
	// remove it, so this can't be used to make someone else's (or a built-in
	// row's) greeting disappear out from under them.
	mux.HandleFunc("DELETE /api/directory/greetings", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		email := effectiveEmail(cache, r)
		owned, err := ownsGreeting(source, name, email)
		if err != nil {
			log.Printf("[ERROR] load greetings for delete: %v", err)
			http.Error(w, "failed to delete greeting", http.StatusInternalServerError)
			return
		}
		if !owned {
			http.Error(w, "you can only delete greetings you created", http.StatusForbidden)
			return
		}
		if err := writer.Delete(invitesApp, "_Greetings", map[string]string{"Name": name}); err != nil {
			log.Printf("[ERROR] delete greeting %q: %v", name, err)
			http.Error(w, "failed to delete greeting", http.StatusInternalServerError)
			return
		}
		log.Printf("greeting: %s deleted %q", email, name)
		w.WriteHeader(http.StatusNoContent)
	})
}

// ownsGreeting reports whether the named _Greetings row exists and was
// created by email - the shared check behind both editing and deleting one,
// so neither can be used to take over or remove someone else's greeting (or
// a built-in row, which has no CreatedBy to match at all).
func ownsGreeting(source data.Source, name, email string) (bool, error) {
	greetings, err := loadGreetingTemplates(source)
	if err != nil {
		return false, err
	}
	for _, g := range greetings {
		if strings.EqualFold(g.Name, name) {
			return g.CreatedBy == email, nil
		}
	}
	return false, nil
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}
