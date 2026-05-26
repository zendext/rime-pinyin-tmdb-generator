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

type MovieFetcher interface {
	FetchMovies(ctx context.Context, since time.Time) ([]tmdb.SeriesRecord, error)
}

type Options struct {
	StorePath      string
	MovieStorePath string
	DictPath       string
	Mode           string
	Languages      []string
	Fetcher        Fetcher
	MovieFetcher   MovieFetcher
	Encoder        rime.Encoder
	Overrides      map[string]string
	Now            func() time.Time
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
	tvResult, err := generateSeriesDictionaries(ctx, seriesGenerateOptions{
		StorePath:   opts.StorePath,
		DictPath:    opts.DictPath,
		GroupPrefix: dictionaryPrefix(opts.Mode),
		Languages:   opts.Languages,
		Fetch:       opts.Fetcher.FetchSeries,
		Encoder:     opts.Encoder,
		Overrides:   opts.Overrides,
		Now:         now,
	})
	if err != nil {
		return Result{}, err
	}
	result := Result{
		EntryCount: tvResult.EntryCount,
		DictPath:   opts.DictPath,
		DictPaths:  append([]string(nil), tvResult.DictPaths...),
		StorePath:  opts.StorePath,
	}
	if strings.TrimSpace(opts.Mode) == "full" && opts.MovieFetcher != nil && strings.TrimSpace(opts.MovieStorePath) != "" {
		movieResult, err := generateSeriesDictionaries(ctx, seriesGenerateOptions{
			StorePath:   opts.MovieStorePath,
			DictPath:    opts.DictPath,
			GroupPrefix: "tmdb_movie",
			Languages:   opts.Languages,
			Fetch:       opts.MovieFetcher.FetchMovies,
			Encoder:     opts.Encoder,
			Overrides:   opts.Overrides,
			Now:         now,
		})
		if err != nil {
			return Result{}, err
		}
		result.EntryCount += movieResult.EntryCount
		result.DictPaths = append(result.DictPaths, movieResult.DictPaths...)
	}
	return result, nil
}

type seriesFetchFunc func(context.Context, time.Time) ([]tmdb.SeriesRecord, error)

type seriesGenerateOptions struct {
	StorePath   string
	DictPath    string
	GroupPrefix string
	Languages   []string
	Fetch       seriesFetchFunc
	Encoder     rime.Encoder
	Overrides   map[string]string
	Now         func() time.Time
}

func generateSeriesDictionaries(ctx context.Context, opts seriesGenerateOptions) (Result, error) {
	now := opts.Now
	if now == nil {
		now = time.Now
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

	records, err := opts.Fetch(ctx, st.LastSuccessfulFetchAt)
	if err != nil {
		return Result{}, err
	}
	selectedGroups := dictionaryGroups(opts.GroupPrefix, opts.Languages)
	groups := tmdb.ExtractChineseTitleGroups(records, selectedGroups)
	dictPaths := make([]string, 0, len(groups))
	written := make(map[string]bool, len(groups))
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
		written[dictPath] = true
		totalEntries += len(words)
	}
	for _, group := range allDictionaryGroups(opts.GroupPrefix) {
		dictPath := dictionaryPath(opts.DictPath, group.Name)
		if written[dictPath] {
			continue
		}
		if err := os.Remove(dictPath); err != nil && !os.IsNotExist(err) {
			return Result{}, err
		}
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

func dictionaryGroups(prefix string, languages []string) []tmdb.TitleGroup {
	all := allDictionaryGroups(prefix)
	if len(languages) == 0 {
		return all
	}
	enabled := make(map[string]bool, len(languages))
	for _, language := range languages {
		if language = strings.ToLower(strings.TrimSpace(language)); language != "" {
			enabled[language] = true
		}
	}
	groups := make([]tmdb.TitleGroup, 0, 2)
	if enabled["zh-cn"] {
		groups = append(groups, all[0])
	}
	if enabled["zh-tw"] || enabled["zh-hk"] {
		groups = append(groups, all[1])
	}
	return groups
}

func allDictionaryGroups(prefix string) []tmdb.TitleGroup {
	return []tmdb.TitleGroup{
		{Name: prefix + "_hans", Languages: []string{"zh-CN"}},
		{Name: prefix + "_hant", Languages: []string{"zh-TW", "zh-HK"}},
	}
}

func dictionaryPrefix(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "popular"
	}
	return "tmdb_" + mode
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
