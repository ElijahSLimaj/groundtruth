package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func voyageServer(t *testing.T, handler http.HandlerFunc) *Voyage {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	v := NewVoyage("test-key")
	v.BaseURL = server.URL
	v.Dims = 4
	return v
}

func TestVoyageEmbed(t *testing.T) {
	ctx := context.Background()

	t.Run("embeds documents preserving input order", func(t *testing.T) {
		v := voyageServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Error("missing bearer token")
			}
			var body struct {
				Model     string   `json:"model"`
				Input     []string `json:"input"`
				InputType string   `json:"input_type"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.InputType != "document" {
				t.Errorf("input_type = %q, want document", body.InputType)
			}
			if body.Model != "voyage-large-2" {
				t.Errorf("model = %q", body.Model)
			}
			fmt.Fprint(w, `{"data": [
				{"index": 1, "embedding": [0.5, 0.5, 0.5, 0.5]},
				{"index": 0, "embedding": [1, 0, 0, 0]}
			]}`)
		})

		vectors, err := v.Embed(ctx, []string{"first", "second"})
		if err != nil {
			t.Fatal(err)
		}
		if vectors[0][0] != 1 || vectors[1][0] != 0.5 {
			t.Fatalf("order not restored from indexes: %v", vectors)
		}
	})

	t.Run("rejects wrong dimensions", func(t *testing.T) {
		v := voyageServer(t, func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"data": [{"index": 0, "embedding": [1, 0]}]}`)
		})
		if _, err := v.Embed(ctx, []string{"x"}); err == nil {
			t.Fatal("wrong dimension count accepted")
		}
	})

	t.Run("rejects missing embeddings", func(t *testing.T) {
		v := voyageServer(t, func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"data": [{"index": 0, "embedding": [1, 0, 0, 0]}]}`)
		})
		if _, err := v.Embed(ctx, []string{"x", "y"}); err == nil {
			t.Fatal("partial response accepted")
		}
	})

	t.Run("surfaces api errors", func(t *testing.T) {
		v := voyageServer(t, func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"detail": "invalid key"}`, http.StatusUnauthorized)
		})
		if _, err := v.Embed(ctx, []string{"x"}); err == nil {
			t.Fatal("non-200 accepted")
		}
	})
}
