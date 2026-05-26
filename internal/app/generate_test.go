package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zendext/rime-pinyin-tmdb-generator/internal/rime"
	"github.com/zendext/rime-pinyin-tmdb-generator/internal/store"
	"github.com/zendext/rime-pinyin-tmdb-generator/internal/tmdb"
)

type fakeFetcher struct {
	records []tmdb.SeriesRecord
	since   time.Time
}

func (f *fakeFetcher) FetchSeries(ctx context.Context, since time.Time) ([]tmdb.SeriesRecord, error) {
	f.since = since
	return f.records, nil
}

func TestGenerateWritesDictionaryAndAdvancesStateAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "series.sqlite")
	outputPath := filepath.Join(dir, "tmdb.dict.yaml")
	staleHantPath := filepath.Join(dir, "tmdb_popular_hant.dict.yaml")
	if err := os.WriteFile(staleHantPath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeFetcher{records: []tmdb.SeriesRecord{{
		ID:   1,
		Name: "虚构剧集",
	}}}

	result, err := Generate(context.Background(), Options{
		StorePath: storePath,
		DictPath:  outputPath,
		Languages: []string{"zh-CN"},
		Fetcher:   fetcher,
		Encoder: fakeEncoder{
			"虚构剧集": "xu gou ju ji",
		},
		Now: func() time.Time { return time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.EntryCount != 1 {
		t.Fatalf("expected 1 entry, got %d", result.EntryCount)
	}
	data, err := os.ReadFile(filepath.Join(dir, "tmdb_popular_hans.dict.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "name: tmdb_popular_hans") {
		t.Fatalf("generated dictionary has wrong name:\n%s", data)
	}
	if !strings.Contains(string(data), "虚构剧集\txu gou ju ji\t90") {
		t.Fatalf("generated dictionary missing expected entry:\n%s", data)
	}
	if _, err := os.Stat(staleHantPath); !os.IsNotExist(err) {
		t.Fatalf("hant dictionary should not be generated for zh-CN-only config, err=%v", err)
	}
	db, err := store.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st, err := db.RunState()
	if err != nil {
		t.Fatal(err)
	}
	if st.EntryCount != 1 || st.LastSuccessfulFetchAt.IsZero() || st.LastGeneratedAt.IsZero() {
		t.Fatalf("unexpected run state: %#v", st)
	}
}

func TestGenerateWritesSeparateHansAndHantDictionaries(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "series.sqlite")
	outputPath := filepath.Join(dir, "tmdb.dict.yaml")
	fetcher := &fakeFetcher{records: []tmdb.SeriesRecord{{
		ID:   1,
		Name: "Imaginary Show",
		Aliases: []tmdb.Alias{
			{Name: "Unlabeled Alias"},
		},
		Translations: map[string]tmdb.Translation{
			"zh-CN": {Name: "虚构剧集"},
			"zh-SG": {Name: "新加坡译名"},
			"zh-TW": {Name: "虛構劇集"},
		},
	}}}

	result, err := Generate(context.Background(), Options{
		StorePath: storePath,
		DictPath:  outputPath,
		Languages: []string{"zh-CN", "zh-TW"},
		Fetcher:   fetcher,
		Encoder: fakeEncoder{
			"虚构剧集": "xu gou ju ji",
			"虛構劇集": "xu gou ju ji",
		},
		Now: func() time.Time { return time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.EntryCount != 2 {
		t.Fatalf("expected 2 entries, got %d", result.EntryCount)
	}

	hansData := mustReadFile(t, filepath.Join(dir, "tmdb_popular_hans.dict.yaml"))
	mustContainText(t, hansData, "name: tmdb_popular_hans")
	mustContainText(t, hansData, "虚构剧集\txu gou ju ji\t100")
	mustNotContainText(t, hansData, "新加坡译名")
	mustNotContainText(t, hansData, "虛構劇集")
	mustNotContainText(t, hansData, "Unlabeled Alias")

	hantData := mustReadFile(t, filepath.Join(dir, "tmdb_popular_hant.dict.yaml"))
	mustContainText(t, hantData, "name: tmdb_popular_hant")
	mustContainText(t, hantData, "虛構劇集\txu gou ju ji\t100")
	mustNotContainText(t, hantData, "虚构剧集")
	mustNotContainText(t, hantData, "Unlabeled Alias")
}

func TestGenerateUsesModeSpecificDictionaryNames(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "series.sqlite")
	outputPath := filepath.Join(dir, "tmdb.dict.yaml")
	fetcher := &fakeFetcher{records: []tmdb.SeriesRecord{{
		ID:   1,
		Name: "虚构剧集",
	}}}

	result, err := Generate(context.Background(), Options{
		StorePath: storePath,
		DictPath:  outputPath,
		Mode:      "full",
		Fetcher:   fetcher,
		Encoder: fakeEncoder{
			"虚构剧集": "xu gou ju ji",
		},
		Now: func() time.Time { return time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.DictPaths) != 2 {
		t.Fatalf("expected 2 dict paths, got %#v", result.DictPaths)
	}
	hansPath := filepath.Join(dir, "tmdb_full_hans.dict.yaml")
	if result.DictPaths[0] != hansPath {
		t.Fatalf("expected first dict path %q, got %#v", hansPath, result.DictPaths)
	}
	hansData := mustReadFile(t, hansPath)
	mustContainText(t, hansData, "name: tmdb_full_hans")
}

func TestGenerateUsesLastSuccessfulFetchAtFromSQLiteStore(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "series.sqlite")
	outputPath := filepath.Join(dir, "tmdb.dict.yaml")
	since := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)

	db, err := store.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveRunState(store.RunState{LastSuccessfulFetchAt: since}); err != nil {
		t.Fatal(err)
	}
	db.Close()

	fetcher := &fakeFetcher{}
	_, err = Generate(context.Background(), Options{
		StorePath: storePath,
		DictPath:  outputPath,
		Fetcher:   fetcher,
		Encoder:   fakeEncoder{},
		Now:       func() time.Time { return time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fetcher.since.Equal(since) {
		t.Fatalf("expected fetch since %s, got %s", since, fetcher.since)
	}
}

type fakeEncoder map[string]string

func (f fakeEncoder) Encode(word string) (string, bool) {
	p, ok := f[word]
	return p, ok
}

var _ rime.Encoder = fakeEncoder{}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustContainText(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Fatalf("expected %q in:\n%s", needle, text)
	}
}

func mustNotContainText(t *testing.T, text, needle string) {
	t.Helper()
	if strings.Contains(text, needle) {
		t.Fatalf("did not expect %q in:\n%s", needle, text)
	}
}
