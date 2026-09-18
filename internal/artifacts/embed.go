package artifacts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"strings"
	"unicode"

	"google.golang.org/api/option"
	transport "google.golang.org/api/transport/http"
)

const (
	vertexBase    = "gemini-embedding-001"
	vertexProject = "heliosian"
	vertexRegion  = "us-west1"
	vertexBatch   = 25
	// vertexDims is the width asked for. The model writes three thousand,
	// nested so that a prefix is itself a good vector; a few hundred is as
	// accurate for a corpus this size and a quarter of the bytes to keep,
	// fetch and hold.
	vertexDims = 768
	fakeDims   = 256
)

// An Embedder turns text into vectors: the documents' chunks when they are
// imported, and a question when one is asked. Model names the embedding, and
// a document embedded by one model is never compared against another's.
type Embedder interface {
	Embed(ctx context.Context, texts []string, query bool) ([]Vector, error)
	Model() string
}

// A Vector is one chunk's embedding, written as base64 little-endian
// float32: a document of a hundred chunks is then hundreds of kilobytes
// rather than the megabytes the same numbers as decimal text would take.
type Vector []float32

func (v Vector) MarshalJSON() ([]byte, error) {
	raw := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(raw[4*i:], math.Float32bits(x))
	}
	return json.Marshal(base64.StdEncoding.EncodeToString(raw))
}

func (v *Vector) UnmarshalJSON(data []byte) error {
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	if len(raw)%4 != 0 {
		return fmt.Errorf("a vector of %d bytes is not float32", len(raw))
	}
	out := make(Vector, len(raw)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:]))
	}
	*v = out
	return nil
}

// Vertex embeds through Gemini's embedding model on Vertex AI, as the
// Google credentials at hand.
type Vertex struct {
	client *http.Client
}

func NewVertex() (*Vertex, error) {
	client, _, err := transport.NewClient(context.Background(), option.WithScopes("https://www.googleapis.com/auth/cloud-platform"))
	if err != nil {
		return nil, fmt.Errorf("vertex client: %w", err)
	}
	return &Vertex{client: client}, nil
}

func (Vertex) Model() string {
	return fmt.Sprintf("%s@%d", vertexBase, vertexDims)
}

func (v *Vertex) Embed(ctx context.Context, texts []string, query bool) ([]Vector, error) {
	out := []Vector{}
	for start := 0; start < len(texts); start += vertexBatch {
		end := min(start+vertexBatch, len(texts))
		vectors, err := v.predict(ctx, texts[start:end], query)
		if err != nil {
			return nil, err
		}
		out = append(out, vectors...)
	}
	return out, nil
}

func (v *Vertex) predict(ctx context.Context, texts []string, query bool) ([]Vector, error) {
	task := "RETRIEVAL_DOCUMENT"
	if query {
		task = "RETRIEVAL_QUERY"
	}
	instances := []map[string]string{}
	for _, text := range texts {
		instances = append(instances, map[string]string{"content": text, "task_type": task})
	}
	body, err := json.Marshal(map[string]any{"instances": instances, "parameters": map[string]any{"outputDimensionality": vertexDims}})
	if err != nil {
		return nil, err
	}
	address := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:predict", vertexRegion, vertexProject, vertexRegion, vertexBase)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed: %s: %s", resp.Status, answer)
	}
	var parsed struct {
		Predictions []struct {
			Embeddings struct {
				Values []float32 `json:"values"`
			} `json:"embeddings"`
		} `json:"predictions"`
		Metadata struct {
			BillableCharacterCount int `json:"billableCharacterCount"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(answer, &parsed); err != nil {
		return nil, fmt.Errorf("embed: read the answer: %w", err)
	}
	if len(parsed.Predictions) != len(texts) {
		return nil, fmt.Errorf("embed: %d texts sent, %d vectors returned", len(texts), len(parsed.Predictions))
	}
	Billed += parsed.Metadata.BillableCharacterCount
	out := []Vector{}
	for i, p := range parsed.Predictions {
		if len(p.Embeddings.Values) != vertexDims {
			return nil, fmt.Errorf("embed: vector %d has %d dimensions, not %d", i, len(p.Embeddings.Values), vertexDims)
		}
		out = append(out, p.Embeddings.Values)
	}
	return out, nil
}

// Billed is how many characters the model has charged for this run, for the
// import to report what it spent.
var Billed int

// Fake stands in for the model in sample mode and in the tests: a hashed bag
// of the words, which finds the right passage for a plain question without
// asking anything of anyone.
type Fake struct{}

func (Fake) Model() string {
	return fmt.Sprintf("fake-bag-of-words@%d", fakeDims)
}

func (Fake) Embed(ctx context.Context, texts []string, query bool) ([]Vector, error) {
	out := []Vector{}
	for _, text := range texts {
		vector := make(Vector, fakeDims)
		for _, token := range tokens(text) {
			h := fnv.New32a()
			h.Write([]byte(token))
			vector[h.Sum32()%fakeDims]++
		}
		Normalize(vector)
		out = append(out, vector)
	}
	return out, nil
}

func tokens(text string) []string {
	out := []string{}
	for _, field := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(field) >= 2 {
			out = append(out, field)
		}
	}
	return out
}

func Normalize(vector []float32) {
	sum := 0.0
	for _, x := range vector {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return
	}
	scale := float32(1 / math.Sqrt(sum))
	for i := range vector {
		vector[i] *= scale
	}
}

func dot(a, b []float32) float64 {
	sum := 0.0
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}
