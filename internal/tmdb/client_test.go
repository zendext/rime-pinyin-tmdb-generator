package tmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
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

func TestClientFetchesDiscoverTVAndTranslations(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireAPIKey(t, r.URL.Query())
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/discover/tv":
			if r.URL.Query().Get("language") != "zh-CN" {
				t.Fatalf("unexpected language: %s", r.URL.Query().Get("language"))
			}
			writeJSON(t, w, map[string]any{
				"page":        1,
				"total_pages": 1,
				"results": []map[string]any{{
					"id":            101,
					"name":          "Imaginary Show",
					"original_name": "Imaginary Show",
				}},
			})
		case "/tv/101/translations":
			writeJSON(t, w, map[string]any{
				"id": 101,
				"translations": []map[string]any{{
					"iso_639_1":  "zh",
					"iso_3166_1": "CN",
					"data": map[string]any{
						"name": "虚构剧集",
					},
				}},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	got, err := (&Client{
		BaseURL:   server.URL,
		APIKey:    "test-key",
		Languages: []string{"zh-CN"},
		MaxPages:  1,
		HTTP:      server.Client(),
	}).FetchSeries(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	want := []SeriesRecord{{
		ID:   101,
		Name: "Imaginary Show",
		Translations: map[string]Translation{
			"zh-CN": {Name: "虚构剧集"},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("records mismatch\nwant: %#v\n got: %#v", want, got)
	}
	if !reflect.DeepEqual(paths, []string{"/discover/tv", "/tv/101/translations"}) {
		t.Fatalf("unexpected request paths: %#v", paths)
	}
}

func TestClientFetchesChangedTVIDsSinceLastRun(t *testing.T) {
	var paths []string
	since := time.Now().UTC().AddDate(0, 0, -1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireAPIKey(t, r.URL.Query())
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/tv/changes":
			if r.URL.Query().Get("start_date") != since.Format("2006-01-02") {
				t.Fatalf("unexpected start date: %s", r.URL.Query().Get("start_date"))
			}
			writeJSON(t, w, map[string]any{
				"page":        1,
				"total_pages": 1,
				"results": []map[string]any{{
					"id": 202,
				}},
			})
		case "/tv/202":
			writeJSON(t, w, map[string]any{
				"id":            202,
				"name":          "更新剧集",
				"original_name": "Updated Show",
			})
		case "/tv/202/translations":
			writeJSON(t, w, map[string]any{
				"id":           202,
				"translations": []map[string]any{},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	got, err := (&Client{
		BaseURL:   server.URL,
		APIKey:    "test-key",
		Languages: []string{"zh-CN"},
		MaxPages:  1,
		HTTP:      server.Client(),
	}).FetchSeries(context.Background(), since)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != 202 || got[0].Name != "更新剧集" {
		t.Fatalf("unexpected records: %#v", got)
	}
	if !reflect.DeepEqual(paths, []string{"/tv/changes", "/tv/202", "/tv/202/translations"}) {
		t.Fatalf("unexpected request paths: %#v", paths)
	}
}

func TestClientSplitsChangedTVQueriesIntoFourteenDayWindows(t *testing.T) {
	start := time.Now().UTC().AddDate(0, 0, -20)
	var ranges [][2]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireAPIKey(t, r.URL.Query())
		switch r.URL.Path {
		case "/tv/changes":
			ranges = append(ranges, [2]string{r.URL.Query().Get("start_date"), r.URL.Query().Get("end_date")})
			writeJSON(t, w, map[string]any{
				"page":        1,
				"total_pages": 1,
				"results": []map[string]any{{
					"id": 202,
				}},
			})
		case "/tv/202":
			writeJSON(t, w, map[string]any{
				"id":            202,
				"name":          "更新剧集",
				"original_name": "Updated Show",
			})
		case "/tv/202/translations":
			writeJSON(t, w, map[string]any{
				"id":           202,
				"translations": []map[string]any{},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	_, err := (&Client{
		BaseURL:   server.URL,
		APIKey:    "test-key",
		Languages: []string{"zh-CN"},
		MaxPages:  1,
		HTTP:      server.Client(),
	}).FetchSeries(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}

	if len(ranges) != 2 {
		t.Fatalf("expected 2 change windows, got %#v", ranges)
	}
	firstStart, err := time.Parse("2006-01-02", ranges[0][0])
	if err != nil {
		t.Fatal(err)
	}
	firstEnd, err := time.Parse("2006-01-02", ranges[0][1])
	if err != nil {
		t.Fatal(err)
	}
	if firstEnd.Sub(firstStart) > 14*24*time.Hour {
		t.Fatalf("first change window exceeds 14 days: %#v", ranges[0])
	}
}

func requireAPIKey(t *testing.T, q url.Values) {
	t.Helper()
	if q.Get("api_key") != "test-key" {
		t.Fatalf("unexpected api_key: %q", q.Get("api_key"))
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
