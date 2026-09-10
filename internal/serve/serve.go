// Package serve sends files with a content ETag and no modification time, so a conditional request only matches identical bytes.
package serve

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func File(w http.ResponseWriter, r *http.Request, name string) {
	data, err := os.ReadFile(name)
	if err != nil {
		slog.ErrorContext(r.Context(), "read file", "name", name, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	sum := sha256.Sum256(data)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:])+`"`)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}
