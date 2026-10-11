package model

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"heliosian/internal/auth"
)

func TestWidgetsAnswerAParent(t *testing.T) {
	_, _, mux := homeServer(t)
	for _, path := range []string{"/api/home-widgets", "/api/events?app=celebrate&from=2026-09-17", "/api/school-emails", "/api/alerts", "/api/home-settings?include=viewer", "/api/apps", "/api/links?include=category", "/api/link-categories?include=links"} {
		rec := httptest.NewRecorder()
		auth.Fixed(jordan, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: %d %s", path, rec.Code, rec.Body)
		}
	}
}

func TestACategoryTakesAnUploadedIcon(t *testing.T) {
	_, _, mux := homeServer(t)
	var pic bytes.Buffer
	if err := png.Encode(&pic, image.NewGray(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("image", "icon.png")
	if err != nil {
		t.Fatal(err)
	}
	part.Write(pic.Bytes())
	form.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/apps/image", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	auth.Fixed(homeAdmin, mux).ServeHTTP(rec, req)
	var made struct {
		Name string `json:"name"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &made) != nil || made.Name == "" {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	write(t, mux, homeAdmin, "POST", "/api/link-categories/"+schoolID+"/edit", map[string]any{"emoji": "image:" + made.Name})
	got := read(t, mux, homeAdmin, "/api/link-categories/"+schoolID).one(t, "link-categories", schoolID)
	if got["emoji"] != "image:"+made.Name || got["imageUrl"] != "/"+made.Name {
		t.Errorf("after saving an uploaded icon the category reads %v", got)
	}
}

func TestOnlyAdminsAddImages(t *testing.T) {
	c, _, mux := homeServer(t)
	const member = "robin.whitfield@heliosschool.org"
	admins := c.Model().AdminList("home")
	if !admins.IsAdmin(homeAdmin) || admins.IsAdmin(member) {
		t.Fatalf("sample admins: %s %v, %s %v", homeAdmin, admins.IsAdmin(homeAdmin), member, admins.IsAdmin(member))
	}

	var pic bytes.Buffer
	if err := png.Encode(&pic, image.NewGray(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	upload := func(as string) int {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, err := form.CreateFormFile("image", "picture.png")
		if err != nil {
			t.Fatal(err)
		}
		part.Write(pic.Bytes())
		form.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/apps/image", &body)
		req.Header.Set("Content-Type", form.FormDataContentType())
		rec := httptest.NewRecorder()
		auth.Fixed(as, mux).ServeHTTP(rec, req)
		return rec.Code
	}
	if code := upload(member); code != http.StatusForbidden {
		t.Errorf("member upload: %d, want 403", code)
	}
	if code := upload(homeAdmin); code != http.StatusOK {
		t.Errorf("admin upload: %d, want 200", code)
	}
	for _, path := range []string{"/api/apps/images/search?q=soccer", "/api/apps/images/thumb?id=x"} {
		rec := httptest.NewRecorder()
		auth.Fixed(member, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusForbidden {
			t.Errorf("member %s: %d, want 403", path, rec.Code)
		}
	}
}
