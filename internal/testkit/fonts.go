package testkit

import (
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
)

const (
	GoogleFontsHost      = "fonts.googleapis.com"
	GoogleFontsFilesHost = "fonts.gstatic.com"
)

func GoogleFonts(fetched *atomic.Int32) (http.Handler, http.Handler) {
	css := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprintf(w, "@font-face { font-family: %q; src: url(https://%s/s/sample.ttf) format('truetype'); }\n", r.URL.Query().Get("family"), GoogleFontsFilesHost)
	})
	files := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile("web/common/fonts/Montserrat-Medium.ttf")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fetched.Add(1)
		w.Header().Set("Content-Type", "font/ttf")
		w.Write(data)
	})
	return css, files
}
