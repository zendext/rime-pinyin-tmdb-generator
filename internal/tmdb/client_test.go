package tmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestClientRequiresAPIKey(t *testing.T) {
	_, err := (&Client{}).FetchSeries(context.Background(), time.Time{})
	if err == nil || err.Error() != "TMDb API key is required" {
		t.Fatalf("expected missing API key error, got %v", err)
	}
}

func TestClientRejectsUnknownBootstrapMode(t *testing.T) {
	_, err := (&Client{
		APIKey:        "test-key",
		BootstrapMode: "legacy",
	}).FetchSeries(context.Background(), time.Time{})
	if err == nil || err.Error() != `unsupported bootstrap mode "legacy"` {
		t.Fatalf("expected unsupported bootstrap mode error, got %v", err)
	}
}

func TestClientRejectsPopularBootstrapMode(t *testing.T) {
	_, err := (&Client{
		APIKey:        "test-key",
		BootstrapMode: "popular",
	}).FetchSeries(context.Background(), time.Time{})
	if err == nil || err.Error() != `unsupported bootstrap mode "popular"` {
		t.Fatalf("expected unsupported bootstrap mode error, got %v", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func pathID(t *testing.T, path string) int {
	t.Helper()
	parts := strings.Split(path, "/")
	if len(parts) < 5 {
		t.Fatalf("unexpected translation path: %s", path)
	}
	id, err := strconv.Atoi(parts[3])
	if err != nil {
		t.Fatalf("unexpected translation path: %s", path)
	}
	return id
}

func translationPayload(id int, name string) map[string]any {
	return map[string]any{
		"id": id,
		"translations": []map[string]any{{
			"iso_639_1":  "zh",
			"iso_3166_1": "CN",
			"data": map[string]any{
				"name": name,
			},
		}},
	}
}
