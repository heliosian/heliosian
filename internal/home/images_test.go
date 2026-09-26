package home

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/imagesearch"
)

func TestOnlyAdminsAddImages(t *testing.T) {
	c, _ := sampleCache(t)
	const member = "robin.whitfield@heliosschool.org"
	if !c.IsAdmin(admin) || c.IsAdmin(member) {
		t.Fatalf("sample admins: %s %v, %s %v", admin, c.IsAdmin(admin), member, c.IsAdmin(member))
	}
	store := blob.NewMemory()
	search := imagesearch.Search{Stock: imagesearch.NewStock(store), Limits: imagesearch.NewLimits()}
	mux := http.NewServeMux()
	Register(mux, c, store, func() []string { return nil }, nil, nil, directoryOf(t), nil, nil, nil, search, nil, nil)

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
	if code := upload(admin); code != http.StatusOK {
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
