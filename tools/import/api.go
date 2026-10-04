package main

import (
	"bytes"
	"mime/multipart"
	"net/http"

	"heliosian/internal/qclient"
)

type client struct {
	qclient.Client
}

func (c client) table(name string, where ...any) (table, string, error) {
	q := map[string]any{"from": name}
	if len(where) > 0 {
		q["where"] = where
	}
	a, err := c.Query(q)
	if err != nil {
		return table{}, "", err
	}
	t := table{rows: map[string]row{}}
	for _, id := range a.Result {
		t.add(id, a.Resources[name][id])
	}
	return t, a.Now, nil
}

func in(path string, values ...any) map[string]any {
	return map[string]any{"in": append([]any{map[string]any{"path": path}}, values...)}
}

func or(conditions ...any) map[string]any {
	return map[string]any{"or": conditions}
}

func roleGroup(kind, slug string) map[string]any {
	slugs := []any{"everyone"}
	for _, rg := range roleGroups {
		slugs = append(slugs, rg.slug)
	}
	return map[string]any{"and": []any{in(kind, "group"), in(slug, slugs...)}}
}

func (c client) read() (*state, error) {
	st := &state{}
	var err error
	if st.people, st.now, err = c.table("PERSON"); err != nil {
		return nil, err
	}
	if st.emails, _, err = c.table("PERSON_EMAIL"); err != nil {
		return nil, err
	}
	if st.photos, _, err = c.table("PHOTO"); err != nil {
		return nil, err
	}
	if st.groups, _, err = c.table("GROUP", or(in("kind", "family", "classroom", "crew", "grade", "band", "department"), roleGroup("kind", "slug"))); err != nil {
		return nil, err
	}
	if st.members, _, err = c.table("MEMBER", or(in("group.kind", "family"), roleGroup("group.kind", "group.slug"))); err != nil {
		return nil, err
	}
	if st.rules, _, err = c.table("RULE", in("group.kind", "band")); err != nil {
		return nil, err
	}
	return st, nil
}

func (c client) write(batch []write) ([]string, error) {
	return c.Write(map[string]any{"batch": batch})
}

func (c client) addPhoto(p portrait) error {
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	if err := form.WriteField("person", p.person); err != nil {
		return err
	}
	part, err := form.CreateFormFile("photo", p.name)
	if err != nil {
		return err
	}
	if _, err := part.Write(p.content); err != nil {
		return err
	}
	if err := form.Close(); err != nil {
		return err
	}
	var out struct {
		Result []string `json:"result"`
	}
	return c.Send(http.MethodPost, "/api/do/photo", form.FormDataContentType(), body, &out)
}
