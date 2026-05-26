package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestClientPopularBootstrapUsesCommonListsWithoutOnTheAir(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/3/trending/tv/week":
			writeJSON(t, w, map[string]any{
				"page":        1,
				"total_pages": 1,
				"results": []map[string]any{{
					"id":         101,
					"popularity": 10.5,
				}},
			})
		case "/3/tv/popular":
			writeJSON(t, w, map[string]any{
				"page":        1,
				"total_pages": 1,
				"results": []map[string]any{{
					"id":         101,
					"popularity": 10.5,
				}},
			})
		case "/3/tv/top_rated":
			writeJSON(t, w, map[string]any{
				"page":        1,
				"total_pages": 1,
				"results": []map[string]any{{
					"id":         202,
					"popularity": 5.5,
				}},
			})
		case "/3/tv/101/translations":
			writeJSON(t, w, translationPayload(101, "虚构剧集"))
		case "/3/tv/202/translations":
			writeJSON(t, w, translationPayload(202, "高分剧集"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{
		BaseURL:         server.URL + "/3",
		APIKey:          "test-key",
		Languages:       []string{"zh-CN"},
		MaxPages:        1,
		HTTP:            server.Client(),
		BootstrapMode:   "popular",
		StorePath:       filepath.Join(t.TempDir(), "series.sqlite"),
		RequestInterval: 0,
	}

	got, err := client.FetchSeries(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []SeriesRecord{
		{ID: 101, Translations: map[string]Translation{"zh-CN": {Name: "虚构剧集"}}},
		{ID: 202, Translations: map[string]Translation{"zh-CN": {Name: "高分剧集"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("records mismatch\nwant: %#v\n got: %#v", want, got)
	}
	wantPaths := []string{
		"/3/trending/tv/week",
		"/3/tv/popular",
		"/3/tv/top_rated",
		"/3/tv/101/translations",
		"/3/tv/202/translations",
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("unexpected paths: %#v", paths)
	}
}

func TestClientPopularBootstrapUsesSourceSpecificPageLimits(t *testing.T) {
	pageCounts := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/trending/tv/week", "/3/tv/popular", "/3/tv/top_rated":
			page, err := strconv.Atoi(r.URL.Query().Get("page"))
			if err != nil {
				t.Fatal(err)
			}
			pageCounts[r.URL.Path]++
			id := pageID(r.URL.Path, page)
			writeJSON(t, w, map[string]any{
				"page":        page,
				"total_pages": 20,
				"results": []map[string]any{{
					"id":         id,
					"popularity": float64(id),
				}},
			})
		default:
			writeJSON(t, w, translationPayload(pathID(t, r.URL.Path), "虚构剧集"))
		}
	}))
	defer server.Close()

	client := &Client{
		BaseURL:         server.URL + "/3",
		APIKey:          "test-key",
		Languages:       []string{"zh-CN"},
		HTTP:            server.Client(),
		BootstrapMode:   "popular",
		StorePath:       filepath.Join(t.TempDir(), "series.sqlite"),
		RequestInterval: 0,
	}

	if _, err := client.FetchSeries(context.Background(), time.Time{}); err != nil {
		t.Fatal(err)
	}
	if pageCounts["/3/trending/tv/week"] != 5 {
		t.Fatalf("expected 5 trending pages, got %d", pageCounts["/3/trending/tv/week"])
	}
	if pageCounts["/3/tv/popular"] != 10 {
		t.Fatalf("expected 10 popular pages, got %d", pageCounts["/3/tv/popular"])
	}
	if pageCounts["/3/tv/top_rated"] != 10 {
		t.Fatalf("expected 10 top rated pages, got %d", pageCounts["/3/tv/top_rated"])
	}
}

func TestClientPopularBootstrapCapsSourceSpecificPageLimitsWithMaxPages(t *testing.T) {
	pageCounts := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/trending/tv/week", "/3/tv/popular", "/3/tv/top_rated":
			page, err := strconv.Atoi(r.URL.Query().Get("page"))
			if err != nil {
				t.Fatal(err)
			}
			pageCounts[r.URL.Path]++
			id := pageID(r.URL.Path, page)
			writeJSON(t, w, map[string]any{
				"page":        page,
				"total_pages": 20,
				"results": []map[string]any{{
					"id":         id,
					"popularity": float64(id),
				}},
			})
		default:
			writeJSON(t, w, translationPayload(pathID(t, r.URL.Path), "虚构剧集"))
		}
	}))
	defer server.Close()

	client := &Client{
		BaseURL:         server.URL + "/3",
		APIKey:          "test-key",
		Languages:       []string{"zh-CN"},
		MaxPages:        3,
		HTTP:            server.Client(),
		BootstrapMode:   "popular",
		StorePath:       filepath.Join(t.TempDir(), "series.sqlite"),
		RequestInterval: 0,
	}

	if _, err := client.FetchSeries(context.Background(), time.Time{}); err != nil {
		t.Fatal(err)
	}
	for source, got := range pageCounts {
		if got != 3 {
			t.Fatalf("expected %s to be capped at 3 pages, got %d", source, got)
		}
	}
}

func pageID(path string, page int) int {
	switch path {
	case "/3/trending/tv/week":
		return 1000 + page
	case "/3/tv/popular":
		return 2000 + page
	case "/3/tv/top_rated":
		return 3000 + page
	default:
		return page
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
