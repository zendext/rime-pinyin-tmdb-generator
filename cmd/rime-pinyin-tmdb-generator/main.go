package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/zendext/rime-pinyin-tmdb-generator/internal/app"
	"github.com/zendext/rime-pinyin-tmdb-generator/internal/config"
	"github.com/zendext/rime-pinyin-tmdb-generator/internal/lock"
	"github.com/zendext/rime-pinyin-tmdb-generator/internal/pinyin"
	"github.com/zendext/rime-pinyin-tmdb-generator/internal/store"
	"github.com/zendext/rime-pinyin-tmdb-generator/internal/tmdb"
)

const version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "generate":
		return runGenerate(args[1:], stdout, stderr)
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintf(stdout, "rime-pinyin-tmdb-generator %s\n", version)
		return 0
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", config.DefaultPath(), "config.toml path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "store_path=%s\n", cfg.Store.Path)
	db, err := store.Open(cfg.Store.Path)
	if err != nil {
		fmt.Fprintf(stderr, "open store: %v\n", err)
		return 1
	}
	defer db.Close()
	st, err := db.RunState()
	if err != nil {
		fmt.Fprintf(stderr, "load run state: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "last_attempted_run_at=%s\n", formatTime(st.LastAttemptedRunAt))
	fmt.Fprintf(stdout, "last_successful_fetch_at=%s\n", formatTime(st.LastSuccessfulFetchAt))
	fmt.Fprintf(stdout, "last_generated_at=%s\n", formatTime(st.LastGeneratedAt))
	fmt.Fprintf(stdout, "entry_count=%d\n", st.EntryCount)
	count, err := db.SeriesCount()
	if err != nil {
		fmt.Fprintf(stderr, "count store series: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "store_series=%d\n", count)
	bootstrap, ok, err := db.EarliestIncompleteBootstrap()
	if err != nil {
		fmt.Fprintf(stderr, "load bootstrap status: %v\n", err)
		return 1
	}
	if !ok {
		bootstrap, ok, err = db.LatestBootstrap()
		if err != nil {
			fmt.Fprintf(stderr, "load bootstrap status: %v\n", err)
			return 1
		}
	}
	timerReady := false
	if ok {
		timerReady = bootstrap.Completed
		fmt.Fprintf(stdout, "bootstrap_export_date=%s\n", bootstrap.ExportDate)
		fmt.Fprintf(stdout, "bootstrap_cursor=%d\n", bootstrap.Cursor)
		fmt.Fprintf(stdout, "bootstrap_completed=%t\n", bootstrap.Completed)
		fmt.Fprintf(stdout, "bootstrap_updated_at=%s\n", formatTime(bootstrap.UpdatedAt))
	} else {
		timerReady = cfg.Bootstrap.Mode == "" || cfg.Bootstrap.Mode == "popular"
		fmt.Fprintln(stdout, "bootstrap_export_date=")
		fmt.Fprintln(stdout, "bootstrap_cursor=0")
		fmt.Fprintln(stdout, "bootstrap_completed=false")
		fmt.Fprintln(stdout, "bootstrap_updated_at=")
	}
	fmt.Fprintf(stdout, "timer_ready=%t\n", timerReady)
	return 0
}

func runGenerate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", config.DefaultPath(), "config.toml path")
	outputPath := fs.String("output", "", "Rime dictionary output path")
	overridesPath := fs.String("overrides", "", "overrides.yaml path")
	apiKey := fs.String("api-key", "", "TMDb API key; TMDB_API_KEY also works")
	baseURL := fs.String("base-url", "", "TMDb API base URL")
	languages := fs.String("languages", "", "comma-separated TMDb language codes")
	bootstrapMode := fs.String("bootstrap-mode", "", "bootstrap mode: popular or full")
	storePath := fs.String("store", "", "SQLite store path")
	exportDate := fs.String("export-date", "", "TMDb daily export date in MM_DD_YYYY")
	requestInterval := fs.Duration("request-interval", 0, "minimum interval between full bootstrap API requests")
	maxItems := fs.Int("max-items", 0, "maximum full bootstrap items to process in this run")
	redeploy := fs.String("redeploy-cmd", "", "optional shell command to run after successful generation")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}
	if *outputPath != "" {
		cfg.Output.DictPath = *outputPath
	}
	if *overridesPath != "" {
		cfg.Overrides = *overridesPath
	}
	if *apiKey != "" {
		cfg.TMDB.APIKey = *apiKey
	}
	if *baseURL != "" {
		cfg.TMDB.BaseURL = *baseURL
	}
	if *languages != "" {
		cfg.TMDB.Languages = splitCSV(*languages)
	}
	if *bootstrapMode != "" {
		cfg.Bootstrap.Mode = *bootstrapMode
	}
	if *storePath != "" {
		cfg.Store.Path = *storePath
		cfg.Store.PathExplicit = true
	}
	if *exportDate != "" {
		cfg.Bootstrap.ExportDate = *exportDate
	}
	if *requestInterval > 0 {
		cfg.Bootstrap.RequestInterval = config.Duration(*requestInterval)
	}
	if *maxItems > 0 {
		cfg.Bootstrap.MaxItems = *maxItems
	}
	if *redeploy != "" {
		cfg.Rime.RedeployCommand = *redeploy
	}
	config.ApplyModeDefaults(&cfg)

	l, err := lock.Acquire(cfg.Output.LockPath)
	if err != nil {
		fmt.Fprintf(stderr, "acquire lock: %v\n", err)
		return 1
	}
	defer l.Release()

	overrides, err := config.LoadOverrides(cfg.Overrides)
	if err != nil {
		fmt.Fprintf(stderr, "load overrides: %v\n", err)
		return 1
	}
	client := &tmdb.Client{
		BaseURL:         cfg.TMDB.BaseURL,
		APIKey:          cfg.TMDB.APIKey,
		Languages:       cfg.TMDB.Languages,
		BootstrapMode:   cfg.Bootstrap.Mode,
		PopularSources:  popularSources(cfg.Bootstrap.Popular),
		ExportBaseURL:   cfg.Bootstrap.ExportBaseURL,
		ExportDate:      cfg.Bootstrap.ExportDate,
		StorePath:       cfg.Store.Path,
		RequestInterval: cfg.Bootstrap.RequestInterval.Std(),
		MaxItems:        cfg.Bootstrap.MaxItems,
		MinPopularity:   cfg.Bootstrap.MinPopularity,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(stdout, format+"\n", args...)
		},
	}
	ctx := context.Background()
	cancel := func() {}
	if cfg.Bootstrap.Mode != "full" {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
	}
	defer cancel()
	result, err := app.Generate(ctx, app.Options{
		StorePath: cfg.Store.Path,
		DictPath:  cfg.Output.DictPath,
		Mode:      cfg.Bootstrap.Mode,
		Languages: cfg.TMDB.Languages,
		Fetcher:   client,
		Encoder:   pinyin.NewEncoder(),
		Overrides: overrides,
	})
	if err != nil {
		fmt.Fprintf(stderr, "generate: %v\n", err)
		return 1
	}
	if cfg.Rime.RedeployCommand != "" {
		if err := runShell(cfg.Rime.RedeployCommand); err != nil {
			fmt.Fprintf(stderr, "redeploy: %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "generated %d entries at %s\n", result.EntryCount, strings.Join(result.DictPaths, ", "))
	return 0
}

func popularSources(cfg config.PopularConfig) []tmdb.PopularSource {
	return []tmdb.PopularSource{
		{Path: "/trending/tv/week", Pages: cfg.TrendingWeekPages},
		{Path: "/tv/popular", Pages: cfg.PopularPages},
		{Path: "/tv/top_rated", Pages: cfg.TopRatedPages},
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: rime-pinyin-tmdb-generator <generate|status|version>")
	fmt.Fprintln(w, "  generate  fetch TMDb metadata locally and write mode-specific dictionaries")
	fmt.Fprintln(w, "  status    print local bootstrap and dictionary status")
	fmt.Fprintln(w, "  version   print version")
}

func splitCSV(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func runShell(command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
