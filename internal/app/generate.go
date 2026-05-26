package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zendext/rime-pinyin-tmdb-generator/internal/rime"
	"github.com/zendext/rime-pinyin-tmdb-generator/internal/store"
	"github.com/zendext/rime-pinyin-tmdb-generator/internal/tmdb"
)

type Fetcher interface {
	FetchSeries(ctx context.Context, since time.Time) ([]tmdb.SeriesRecord, error)
}

type Options struct {
	StorePath string
	DictPath  string
	Languages []string
	Fetcher   Fetcher
	Encoder   rime.Encoder
	Overrides map[string]string
	Now       func() time.Time
}

type Result struct {
	EntryCount int
	DictPath   string
	DictPaths  []string
	StorePath  string
}

func Generate(ctx context.Context, opts Options) (Result, error) {
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	db, err := store.Open(opts.StorePath)
	if err != nil {
		return Result{}, err
	}
	defer db.Close()
	st, err := db.RunState()
	if err != nil {
		return Result{}, err
	}
	st.LastAttemptedRunAt = now().UTC()
	if err := db.SaveRunState(st); err != nil {
		return Result{}, err
	}

	records, err := opts.Fetcher.FetchSeries(ctx, st.LastSuccessfulFetchAt)
	if err != nil {
		return Result{}, err
	}
	groups := tmdb.ExtractChineseTitleGroups(records, dictionaryGroups())
	dictPaths := make([]string, 0, len(groups))
	totalEntries := 0
	for _, group := range groups {
		words := make([]rime.Word, 0, len(group.Titles))
		for _, title := range group.Titles {
			words = append(words, rime.Word{Text: title.Text, Weight: title.Weight})
		}
		dict, err := rime.BuildDictionary(rime.BuildRequest{
			Name:      group.Name,
			Version:   now().Format("2006-01-02"),
			Words:     words,
			Overrides: opts.Overrides,
			Encoder:   opts.Encoder,
		})
		if err != nil {
			return Result{}, err
		}
		dictPath := dictionaryPath(opts.DictPath, group.Name)
		if err := atomicWrite(dictPath, []byte(dict), 0o644); err != nil {
			return Result{}, err
		}
		dictPaths = append(dictPaths, dictPath)
		totalEntries += len(words)
	}

	st.LastSuccessfulFetchAt = now().UTC()
	st.LastGeneratedAt = now().UTC()
	st.EntryCount = totalEntries
	if err := db.SaveRunState(st); err != nil {
		return Result{}, err
	}
	return Result{EntryCount: totalEntries, DictPath: opts.DictPath, DictPaths: dictPaths, StorePath: opts.StorePath}, nil
}

func atomicWrite(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func dictionaryGroups() []tmdb.TitleGroup {
	return []tmdb.TitleGroup{
		{Name: "tmdb_hans", Languages: []string{"zh-CN"}},
		{Name: "tmdb_hant", Languages: []string{"zh-TW", "zh-HK"}},
	}
}

func dictionaryPath(basePath, name string) string {
	dir := filepath.Dir(basePath)
	filename := filepath.Base(basePath)
	suffix := filepath.Ext(filename)
	if strings.HasSuffix(filename, ".dict.yaml") {
		suffix = ".dict.yaml"
	}
	return filepath.Join(dir, name+suffix)
}
