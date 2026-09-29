package model

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

func homeMux(c *Store, s homeSheet) *http.ServeMux {
	bucket := blob.NewMemoryBucket()
	images := blob.New(bucket)
	mux := http.NewServeMux()
	RegisterHome(mux, HomeDeps{
		Store:    c,
		Images:   blob.NewImages(images, "home"),
		Calendar: homeCalendar(c, s),
		Search:   imagesearch.Search{Stock: imagesearch.NewStock(bucket, images), Limits: imagesearch.NewLimits()},
	})
	return mux
}

func TestWidgetsAnswerAParent(t *testing.T) {
	c, s := sampleHomeCache(t)
	mux := homeMux(c, s)
	for _, path := range []string{"/api/apps/team", "/api/apps/celebrate", "/api/apps/school"} {
		rec := httptest.NewRecorder()
		auth.Fixed(jordan, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: %d %s", path, rec.Code, rec.Body)
		}
	}
}

func TestOnlyAdminsAddImages(t *testing.T) {
	c, s := sampleHomeCache(t)
	const member = "robin.whitfield@heliosschool.org"
	admins := c.Model().AdminList("home")
	if !admins.IsAdmin(homeAdmin) || admins.IsAdmin(member) {
		t.Fatalf("sample admins: %s %v, %s %v", homeAdmin, admins.IsAdmin(homeAdmin), member, admins.IsAdmin(member))
	}
	mux := homeMux(c, s)

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
