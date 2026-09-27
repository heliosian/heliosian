package serve

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"heliosian/internal/access"
)

const bodyLimit = 256 << 10

type None struct{}

func File(w http.ResponseWriter, r *http.Request, name string) {
	data, err := os.ReadFile(name)
	if err != nil {
		slog.ErrorContext(r.Context(), "read file", "name", name, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	Content(w, r, name, data)
}

func Content(w http.ResponseWriter, r *http.Request, name string, data []byte) {
	sum := sha256.Sum256(data)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:])+`"`)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func JSON[In, Out any](h func(r *http.Request, in In) (Out, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in In
		if _, none := any(in).(None); !none {
			if !Decode(w, r, &in) {
				return
			}
		}
		out, err := h(r, in)
		if err != nil {
			Error(w, r, err)
			return
		}
		if _, none := any(out).(None); none {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		Write(w, r, http.StatusOK, out)
	}
}

func Decode(w http.ResponseWriter, r *http.Request, into any) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, bodyLimit)).Decode(into)
	if err == nil {
		return true
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return false
	}
	http.Error(w, "bad request body", http.StatusBadRequest)
	return false
}

func Write(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.ErrorContext(r.Context(), "encode response", "path", r.URL.Path, "error", err)
	}
}

func Error(w http.ResponseWriter, r *http.Request, err error) {
	var refusal *access.Refusal
	if !errors.As(err, &refusal) {
		slog.ErrorContext(r.Context(), "request failed", "path", r.URL.Path, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if refusal.Body != nil {
		Write(w, r, refusal.Status, refusal.Body)
		return
	}
	http.Error(w, refusal.Message, refusal.Status)
}

func ID(n int) string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	for i, b := range raw {
		raw[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(raw)
}
