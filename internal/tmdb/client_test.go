package tmdb

import (
	"context"
	"encoding/json"
	"net/http"
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

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
