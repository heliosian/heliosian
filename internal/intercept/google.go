package intercept

import (
	"encoding/json"
	"hash/fnv"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"heliosian/internal/logging"
)

const (
	TokenHost  = "oauth2.googleapis.com"
	VertexHost = "us-west1-aiplatform.googleapis.com"
)

func GoogleLogin(dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logging.Fatal("intercept: google login directory", "error", err)
	}
	path := filepath.Join(dir, "google-credentials.json")
	credentials, err := json.Marshal(map[string]string{
		"type":          "authorized_user",
		"client_id":     "intercept",
		"client_secret": "intercept",
		"refresh_token": "intercept",
		"token_uri":     "https://" + TokenHost + "/token",
	})
	if err != nil {
		logging.Fatal("intercept: google credentials", "error", err)
	}
	if err := os.WriteFile(path, credentials, 0o600); err != nil {
		logging.Fatal("intercept: write google credentials", "error", err)
	}
	if err := os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path); err != nil {
		logging.Fatal("intercept: point at google credentials", "error", err)
	}
	Install(TokenHost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"access_token": "intercept", "token_type": "Bearer", "expires_in": 3600})
	}))
}

func Vertex() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, ":predict") {
			http.Error(w, "intercept answers only a POST to :predict", http.StatusNotFound)
			return
		}
		var req struct {
			Instances []struct {
				Content string `json:"content"`
			} `json:"instances"`
			Parameters struct {
				OutputDimensionality int `json:"outputDimensionality"`
			} `json:"parameters"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		predictions := []any{}
		for _, instance := range req.Instances {
			predictions = append(predictions, map[string]any{"embeddings": map[string]any{"values": bagOfWords(instance.Content, req.Parameters.OutputDimensionality)}})
		}
		writeJSON(w, map[string]any{"predictions": predictions})
	})
}

func bagOfWords(text string, dims int) []float32 {
	vector := make([]float32, dims)
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(word) < 2 {
			continue
		}
		h := fnv.New32a()
		h.Write([]byte(word))
		vector[h.Sum32()%uint32(dims)]++
	}
	sum := 0.0
	for _, x := range vector {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return vector
	}
	scale := float32(1 / math.Sqrt(sum))
	for i := range vector {
		vector[i] *= scale
	}
	return vector
}
