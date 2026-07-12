package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

const (
	voyageDefaultBaseURL = "https://api.voyageai.com/v1"
	voyageDefaultModel   = "voyage-large-2"
	voyageDefaultDims    = 1536
)

type Voyage struct {
	APIKey  string
	Model   string
	Dims    int
	BaseURL string

	httpClient *http.Client
	limiter    *rate.Limiter
}

func NewVoyage(apiKey string) *Voyage {
	return &Voyage{
		APIKey:     apiKey,
		Model:      voyageDefaultModel,
		Dims:       voyageDefaultDims,
		BaseURL:    voyageDefaultBaseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		limiter:    rate.NewLimiter(rate.Every(time.Second/5), 5),
	}
}

func (v *Voyage) ModelID() string {
	return v.Model
}

func (v *Voyage) Dimensions() int {
	return v.Dims
}

func (v *Voyage) Embed(ctx context.Context, batch []string) ([][]float32, error) {
	if len(batch) == 0 {
		return nil, nil
	}
	if err := v.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(map[string]any{
		"model":      v.Model,
		"input":      batch,
		"input_type": "document",
	})
	if err != nil {
		return nil, fmt.Errorf("voyage encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		v.BaseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("voyage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+v.APIKey)
	req.Header.Set("content-type", "application/json")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage embeddings: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("voyage read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		limit := len(body)
		if limit > 200 {
			limit = 200
		}
		return nil, fmt.Errorf("voyage embeddings: status %d: %s", resp.StatusCode, body[:limit])
	}

	var decoded struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("voyage decode: %w", err)
	}
	if len(decoded.Data) != len(batch) {
		return nil, fmt.Errorf("voyage returned %d embeddings for %d inputs", len(decoded.Data), len(batch))
	}

	vectors := make([][]float32, len(batch))
	for _, item := range decoded.Data {
		if item.Index < 0 || item.Index >= len(batch) {
			return nil, fmt.Errorf("voyage returned out of range index %d", item.Index)
		}
		if len(item.Embedding) != v.Dims {
			return nil, fmt.Errorf("voyage returned %d dimensions, expected %d", len(item.Embedding), v.Dims)
		}
		vectors[item.Index] = item.Embedding
	}
	for i, vector := range vectors {
		if vector == nil {
			return nil, fmt.Errorf("voyage returned no embedding for input %d", i)
		}
	}
	return vectors, nil
}
