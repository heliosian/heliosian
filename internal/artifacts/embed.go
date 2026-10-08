package artifacts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"google.golang.org/api/option"
	transport "google.golang.org/api/transport/http"
)

const (
	vertexBase    = "gemini-embedding-001"
	vertexProject = "heliosian"
	vertexRegion  = "us-west1"
	vertexBatch   = 25
	AskDims       = 768
	SearchDims    = 3072
)

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
	return fmt.Sprintf("%s@%d", vertexBase, AskDims)
}

func (v *Vertex) Embed(ctx context.Context, texts []string, query bool, dims int) ([][]float32, error) {
	out := [][]float32{}
	for start := 0; start < len(texts); start += vertexBatch {
		end := min(start+vertexBatch, len(texts))
		vectors, err := v.predict(ctx, texts[start:end], query, dims)
		if err != nil {
			return nil, err
		}
		out = append(out, vectors...)
	}
	return out, nil
}

func (v *Vertex) predict(ctx context.Context, texts []string, query bool, dims int) ([][]float32, error) {
	task := "RETRIEVAL_DOCUMENT"
	if query {
		task = "RETRIEVAL_QUERY"
	}
	instances := []map[string]string{}
	for _, text := range texts {
		instances = append(instances, map[string]string{"content": text, "task_type": task})
	}
	body, err := json.Marshal(map[string]any{"instances": instances, "parameters": map[string]any{"outputDimensionality": dims}})
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
	}
	if err := json.Unmarshal(answer, &parsed); err != nil {
		return nil, fmt.Errorf("embed: read the answer: %w", err)
	}
	if len(parsed.Predictions) != len(texts) {
		return nil, fmt.Errorf("embed: %d texts sent, %d vectors returned", len(texts), len(parsed.Predictions))
	}
	out := [][]float32{}
	for i, p := range parsed.Predictions {
		if len(p.Embeddings.Values) != dims {
			return nil, fmt.Errorf("embed: vector %d has %d dimensions, not %d", i, len(p.Embeddings.Values), dims)
		}
		out = append(out, p.Embeddings.Values)
	}
	return out, nil
}
