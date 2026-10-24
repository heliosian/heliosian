package main

import (
	"bytes"
	"mime/multipart"
	"net/http"

	"heliosian/internal/qclient"
)

type client struct {
	*qclient.Client
}

func (c client) table(name string, where ...any) (table, string, error) {
	q := []any{"from", name}
	if len(where) > 0 {
		q = append(q, append([]any{"where"}, where...))
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

func name(p string) map[string]any {
	return map[string]any{"n": p}
}

func in(p string, values ...any) []any {
	return append([]any{"in", name(p)}, values...)
}

func or(conditions ...any) []any {
	return append([]any{"or"}, conditions...)
}

func roleGroup(kind, slug string) []any {
	slugs := []any{"everyone"}
	for _, rg := range roleGroups {
		slugs = append(slugs, rg.slug)
	}
	return []any{"and", in(kind, "group"), in(slug, slugs...)}
}

func parentsGroup(kind, slug string) []any {
	return []any{"and", in(kind, "group"), []any{"contains", name(slug), "-parents"}}
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
	if st.contents, _, err = c.table("CONTENT"); err != nil {
		return nil, err
	}
	if st.photos, _, err = c.table("PHOTO"); err != nil {
		return nil, err
	}
	if st.groups, _, err = c.table("GROUP", or(in("kind", "family", "classroom", "crew", "grade", "gradeband", "department"), roleGroup("kind", "slug"), parentsGroup("kind", "slug"))); err != nil {
		return nil, err
	}
	if st.members, _, err = c.table("MEMBER", or(in("group.kind", "family"), roleGroup("group.kind", "group.slug"))); err != nil {
		return nil, err
	}
	if st.rules, _, err = c.table("RULE", or(in("group.kind", "gradeband"), parentsGroup("group.kind", "group.slug"))); err != nil {
		return nil, err
	}
	if st.aliases, _, err = c.table("ALIAS"); err != nil {
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
