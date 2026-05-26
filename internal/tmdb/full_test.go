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

	"github.com/zendext/rime-pinyin-tmdb-generator/internal/store"
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
	wantPaths = append(wantPaths, "/p/exports/tv_series_ids_05_26_2026.json.gz")
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("second run should diff completed store against daily export, paths=%#v", paths)
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
		"full bootstrap download export=05_26_2026",
		"full bootstrap export_stats export=05_26_2026 total=101 fetchable=101 remaining_fetchable=101 skipped=0 adult=0 invalid=0 below_min_popularity=0 min_popularity=0",
		"full bootstrap progress export=05_26_2026 cursor=100 processed=100",
		"full bootstrap completed export=05_26_2026 cursor=101 processed=101 skipped=0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected log %q in:\n%s", want, out)
		}
	}
}

func TestClientFullBootstrapLogsPauseAndSkippedItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/p/exports/tv_series_ids_05_26_2026.json.gz":
			w.Header().Set("Content-Type", "application/gzip")
			gz := gzip.NewWriter(w)
			fmt.Fprintln(gz, `{"id":101,"popularity":1,"adult":true}`)
			fmt.Fprintln(gz, `{"id":102,"popularity":1,"adult":false}`)
			fmt.Fprintln(gz, `{"id":103,"popularity":1,"adult":false}`)
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
		MaxItems:        1,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(&logs, format+"\n", args...)
		},
	}

	if _, err := client.FetchSeries(context.Background(), time.Time{}); err != nil {
		t.Fatal(err)
	}
	out := logs.String()
	for _, want := range []string{
		"full bootstrap skipped export=05_26_2026 cursor=1 skipped=1",
		"full bootstrap paused export=05_26_2026 cursor=2 processed=1 skipped=1 reason=max_items",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected log %q in:\n%s", want, out)
		}
	}
}

func TestClientFullBootstrapSkipsItemsBelowMinPopularity(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/p/exports/tv_series_ids_05_26_2026.json.gz":
			w.Header().Set("Content-Type", "application/gzip")
			gz := gzip.NewWriter(w)
			fmt.Fprintln(gz, `{"id":101,"popularity":9.9,"adult":false}`)
			fmt.Fprintln(gz, `{"id":102,"popularity":10,"adult":false}`)
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
		case "/3/tv/102/translations":
			writeJSON(t, w, translationPayload(102, "达标剧集"))
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
		MinPopularity:   10,
	}

	got, err := client.FetchSeries(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != 102 {
		t.Fatalf("expected only item at popularity threshold, got %#v", got)
	}
	wantPaths := []string{"/p/exports/tv_series_ids_05_26_2026.json.gz", "/3/tv/102/translations"}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("unexpected paths: %#v", paths)
	}
}

func TestClientFullBootstrapLogsFetchableItemCounts(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "series.sqlite")
	db, err := store.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetBootstrap("05_26_2026", 2, false); err != nil {
		t.Fatal(err)
	}
	db.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/p/exports/tv_series_ids_05_26_2026.json.gz":
			w.Header().Set("Content-Type", "application/gzip")
			gz := gzip.NewWriter(w)
			fmt.Fprintln(gz, `{"id":0,"popularity":100,"adult":false}`)
			fmt.Fprintln(gz, `{"id":101,"popularity":1,"adult":true}`)
			fmt.Fprintln(gz, `{"id":102,"popularity":9.9,"adult":false}`)
			fmt.Fprintln(gz, `{"id":103,"popularity":10,"adult":false}`)
			fmt.Fprintln(gz, `{"id":104,"popularity":20,"adult":false}`)
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
		case "/3/tv/103/translations", "/3/tv/104/translations":
			writeJSON(t, w, translationPayload(pathID(t, r.URL.Path), "达标剧集"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
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
		StorePath:       storePath,
		RequestInterval: 0,
		MinPopularity:   10,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(&logs, format+"\n", args...)
		},
	}

	if _, err := client.FetchSeries(context.Background(), time.Time{}); err != nil {
		t.Fatal(err)
	}
	want := "full bootstrap export_stats export=05_26_2026 total=5 fetchable=2 remaining_fetchable=2 skipped=3 adult=1 invalid=1 below_min_popularity=1 min_popularity=10"
	if !strings.Contains(logs.String(), want) {
		t.Fatalf("expected log %q in:\n%s", want, logs.String())
	}
}

func TestClientFullBootstrapResumesEarliestIncompleteExportBeforeTryingNewDates(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "series.sqlite")
	db, err := store.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetBootstrap("05_25_2026", 1, false); err != nil {
		t.Fatal(err)
	}
	db.Close()

	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/p/exports/tv_series_ids_05_25_2026.json.gz":
			w.Header().Set("Content-Type", "application/gzip")
			gz := gzip.NewWriter(w)
			fmt.Fprintln(gz, `{"id":101,"popularity":1,"adult":false}`)
			fmt.Fprintln(gz, `{"id":102,"popularity":1,"adult":false}`)
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
		case "/3/tv/102/translations":
			writeJSON(t, w, translationPayload(102, "续跑剧集"))
		default:
			http.NotFound(w, r)
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
		StorePath:       storePath,
		RequestInterval: 0,
	}

	got, err := client.FetchSeries(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != 102 {
		t.Fatalf("expected resumed record 102, got %#v", got)
	}
	if len(paths) == 0 || paths[0] != "/p/exports/tv_series_ids_05_25_2026.json.gz" {
		t.Fatalf("expected first request to resume incomplete export, got paths %#v", paths)
	}
}

func TestClientFullCompletedBootstrapDiffsDailyExport(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "series.sqlite")
	db, err := store.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	for _, series := range []store.Series{
		{ID: 101, Popularity: 10, Titles: map[string]string{"zh-CN": "保留剧集"}, FetchedAt: now, LastSeenAt: now},
		{ID: 102, Popularity: 12, Titles: map[string]string{"zh-CN": "流行度变化剧集"}, FetchedAt: now, LastSeenAt: now},
		{ID: 103, Popularity: 13, Titles: map[string]string{"zh-CN": "消失剧集"}, FetchedAt: now, LastSeenAt: now},
		{ID: 104, Popularity: 14, Titles: map[string]string{"zh-CN": "低流行度剧集"}, FetchedAt: now, LastSeenAt: now},
	} {
		if err := db.UpsertSeries(series); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetBootstrap("05_25_2026", 4, true); err != nil {
		t.Fatal(err)
	}
	db.Close()

	var paths []string
	var logs bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/p/exports/tv_series_ids_05_26_2026.json.gz":
			w.Header().Set("Content-Type", "application/gzip")
			gz := gzip.NewWriter(w)
			fmt.Fprintln(gz, `{"id":0,"popularity":100,"adult":false}`)
			fmt.Fprintln(gz, `{"id":101,"popularity":10,"adult":false}`)
			fmt.Fprintln(gz, `{"id":102,"popularity":20,"adult":false}`)
			fmt.Fprintln(gz, `{"id":104,"popularity":9.9,"adult":false}`)
			fmt.Fprintln(gz, `{"id":105,"popularity":30,"adult":true}`)
			fmt.Fprintln(gz, `{"id":106,"popularity":11,"adult":false}`)
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
		case "/3/tv/106/translations":
			writeJSON(t, w, translationPayload(106, "新增剧集"))
		case "/3/tv/102/translations", "/3/tv/101/translations":
			t.Fatalf("existing series should not refetch translations: %s", r.URL.Path)
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
		StorePath:       storePath,
		RequestInterval: 0,
		MinPopularity:   10,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(&logs, format+"\n", args...)
		},
	}

	got, err := client.FetchSeries(context.Background(), time.Date(2026, 5, 25, 13, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := []SeriesRecord{
		{ID: 101, Translations: map[string]Translation{"zh-CN": {Name: "保留剧集"}}},
		{ID: 102, Translations: map[string]Translation{"zh-CN": {Name: "流行度变化剧集"}}},
		{ID: 106, Translations: map[string]Translation{"zh-CN": {Name: "新增剧集"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("records mismatch\nwant: %#v\n got: %#v", want, got)
	}
	wantPaths := []string{"/p/exports/tv_series_ids_05_26_2026.json.gz", "/3/tv/106/translations"}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("unexpected paths: %#v", paths)
	}
	if !strings.Contains(logs.String(), "full export diff export=05_26_2026 added=1 removed=2 popularity_updated=1 unchanged=1") {
		t.Fatalf("missing diff log:\n%s", logs.String())
	}

	db, err = store.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.AllSeries()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[1].ID != 102 || rows[1].Popularity != 20 {
		t.Fatalf("unexpected stored rows: %#v", rows)
	}
	for _, row := range rows {
		if row.ID == 103 || row.ID == 104 {
			t.Fatalf("removed or below-threshold series remained in store: %#v", rows)
		}
	}
}
