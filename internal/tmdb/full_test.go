package tmdb

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClientFullBootstrapUsesDailyExportStoreAndTranslationsEndpoint(t *testing.T) {
	var paths []string
	tvAttempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/p/exports/tv_series_ids_05_26_2026.json.gz":
			w.Header().Set("Content-Type", "application/gzip")
			gz := gzip.NewWriter(w)
			fmt.Fprintln(gz, `{"id":101,"popularity":10.5,"adult":false}`)
			fmt.Fprintln(gz, `{"id":102,"popularity":1.5,"adult":true}`)
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
		case "/3/tv/101/translations":
			tvAttempts++
			if tvAttempts == 1 {
				w.Header().Set("Retry-After", "0")
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
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

	client := &Client{
		BaseURL:         server.URL + "/3",
		APIKey:          "test-key",
		Languages:       []string{"zh-CN"},
		HTTP:            server.Client(),
		BootstrapMode:   "full",
		ExportBaseURL:   server.URL + "/p/exports",
		ExportDate:      "05_26_2026",
		StorePath:       filepath.Join(t.TempDir(), "series.sqlite"),
		RequestInterval: 0,
	}

	got, err := client.FetchSeries(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []SeriesRecord{{
		ID: 101,
		Translations: map[string]Translation{
			"zh-CN": {Name: "虚构剧集"},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("records mismatch\nwant: %#v\n got: %#v", want, got)
	}
	wantPaths := []string{"/p/exports/tv_series_ids_05_26_2026.json.gz", "/3/tv/101/translations", "/3/tv/101/translations"}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("unexpected paths: %#v", paths)
	}

	got, err = client.FetchSeries(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected stored record on second run, got %#v", got)
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("second run should use completed store without refetching, paths=%#v", paths)
	}
}

func TestClientFullBootstrapLogsProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/p/exports/tv_series_ids_05_26_2026.json.gz":
			w.Header().Set("Content-Type", "application/gzip")
			gz := gzip.NewWriter(w)
			for id := 1; id <= 101; id++ {
				fmt.Fprintf(gz, `{"id":%d,"popularity":1,"adult":false}`+"\n", id)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
		default:
			writeJSON(t, w, translationPayload(pathID(t, r.URL.Path), "虚构剧集"))
		}
	}))
	defer server.Close()

	var logs bytes.Buffer
	client := &Client{
		BaseURL:         server.URL + "/3",
		APIKey:          "test-key",
		Languages:       []string{"zh-CN"},
		HTTP:            server.Client(),
		BootstrapMode:   "full",
		ExportBaseURL:   server.URL + "/p/exports",
		ExportDate:      "05_26_2026",
		StorePath:       filepath.Join(t.TempDir(), "series.sqlite"),
		RequestInterval: 0,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(&logs, format+"\n", args...)
		},
	}

	if _, err := client.FetchSeries(context.Background(), time.Time{}); err != nil {
		t.Fatal(err)
	}
	out := logs.String()
	for _, want := range []string{
		"full bootstrap export=05_26_2026 cursor=0 completed=false",
		"full bootstrap progress export=05_26_2026 cursor=100 processed=100",
		"full bootstrap completed export=05_26_2026 cursor=101 processed=101",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected log %q in:\n%s", want, out)
		}
	}
}
