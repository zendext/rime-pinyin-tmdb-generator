package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zendext/rime-pinyin-tmdb-generator/internal/store"
)

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "rime-pinyin-tmdb-generator") {
		t.Fatalf("unexpected version output: %s", stdout.String())
	}
}

func TestRunStatusReportsFullBootstrapIncomplete(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	storePath := filepath.Join(dir, "series.sqlite")
	configData := []byte("[tmdb]\napi_key = \"test\"\n[store]\npath = \"" + storePath + "\"\n")
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetBootstrap("05_26_2026", 123, false); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveRunState(store.RunState{EntryCount: 7}); err != nil {
		t.Fatal(err)
	}
	db.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "--config", configPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"store_series=0",
		"entry_count=7",
		"bootstrap_export_date=05_26_2026",
		"bootstrap_cursor=123",
		"bootstrap_completed=false",
		"timer_ready=false",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in status output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "state_path=") {
		t.Fatalf("status should not print JSON state path:\n%s", out)
	}
}

func TestRunStatusReportsEarliestIncompleteBootstrap(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	storePath := filepath.Join(dir, "series.sqlite")
	configData := []byte("[tmdb]\napi_key = \"test\"\n[store]\npath = \"" + storePath + "\"\n")
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetBootstrap("05_25_2026", 15923, false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetBootstrap("05_26_2026", 108, false); err != nil {
		t.Fatal(err)
	}
	db.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "--config", configPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"bootstrap_export_date=05_25_2026",
		"bootstrap_cursor=15923",
		"bootstrap_completed=false",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in status output:\n%s", want, out)
		}
	}
}

func TestRunStatusRejectsUnknownBootstrapMode(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	storePath := filepath.Join(dir, "series.sqlite")
	configData := []byte("[tmdb]\napi_key = \"test\"\n[store]\npath = \"" + storePath + "\"\n[bootstrap]\nmode = \"legacy\"\n")
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "--config", configPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), `unsupported bootstrap mode "legacy"`) {
		t.Fatalf("expected unsupported mode error, got stdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestRunGenerateRejectsPopularBootstrapModeOverride(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	lockPath := filepath.Join(dir, "update.lock")
	configData := []byte("[tmdb]\napi_key = \"test\"\n[output]\nlock_path = \"" + lockPath + "\"\n")
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"generate", "--config", configPath, "--bootstrap-mode", "popular"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `unsupported bootstrap mode "popular"`) {
		t.Fatalf("expected popular mode rejection, got stdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestRunGenerateWritesFullBootstrapLogsToStdout(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	storePath := filepath.Join(dir, "series.sqlite")
	lockPath := filepath.Join(dir, "update.lock")
	configData := []byte("[tmdb]\napi_key = \"test\"\nbase_url = \"http://127.0.0.1:1\"\n[output]\nlock_path = \"" + lockPath + "\"\n[store]\npath = \"" + storePath + "\"\n[bootstrap]\nmode = \"full\"\nexport_date = \"05_26_2026\"\n")
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"generate", "--config", configPath}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected generate to fail against unreachable test URL")
	}
	for _, want := range []string{
		" INFO  tmdb.full.state ",
		"media=tv",
		"export=05_26_2026",
		"cursor=0",
		"completed=false",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected full bootstrap log to include %q, got stdout:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
		}
	}
	if strings.Contains(stdout.String(), "full bootstrap export=") {
		t.Fatalf("expected full bootstrap log on stdout, got stdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestSystemdTimerRunsDaily(t *testing.T) {
	data, err := os.ReadFile("../../systemd/rime-pinyin-tmdb-generator-update.timer")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "OnCalendar=daily") {
		t.Fatalf("expected daily systemd timer, got:\n%s", string(data))
	}
}

func TestDocsShowSingleDefaultLanguageAndFullStatus(t *testing.T) {
	configData, err := os.ReadFile("../../examples/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	configText := string(configData)
	if !strings.Contains(configText, "languages = [\"zh-CN\"]") {
		t.Fatalf("example config should default to one language:\n%s", configText)
	}
	if strings.Contains(configText, "max_pages") {
		t.Fatalf("example config should not include max_pages:\n%s", configText)
	}
	if !strings.Contains(configText, `dir = "~/.local/share/fcitx5/rime"`) {
		t.Fatalf("example config should use output dir:\n%s", configText)
	}
	if strings.Contains(configText, "dict_path") {
		t.Fatalf("example config should not include old dict_path:\n%s", configText)
	}
	for _, want := range []string{
		"min_popularity = 10",
		"movie_min_popularity = 15",
		"request_interval = \"50ms\"",
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("example config should include %q:\n%s", want, configText)
		}
	}
	if !strings.Contains(configText, "# 可选") || !strings.Contains(configText, "zh-TW") || !strings.Contains(configText, "zh-HK") {
		t.Fatalf("example config should comment optional languages:\n%s", configText)
	}
	if strings.Contains(configText, "state_path") {
		t.Fatalf("example config should not include JSON state_path:\n%s", configText)
	}
	if strings.Contains(configText, "[store]") {
		t.Fatalf("example config should rely on default store path:\n%s", configText)
	}
	if !strings.Contains(configText, "mode = \"full\"") {
		t.Fatalf("example config should default to full mode:\n%s", configText)
	}

	readmeData, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(readmeData)
	if !strings.Contains(readme, "languages = [\"zh-CN\"]") {
		t.Fatal("README should default to one language")
	}
	if !strings.Contains(readme, "mode = \"full\"") || !strings.Contains(readme, "`bootstrap.mode = \"full\"` 是默认模式") {
		t.Fatal("README should document full as the default mode")
	}
	if strings.Contains(readme, "max_pages = 10") {
		t.Fatal("README default config should not include max_pages")
	}
	if strings.Contains(readme, "--max-pages") {
		t.Fatal("README should not document max-pages")
	}
	if !strings.Contains(readme, `dir = "~/.local/share/fcitx5/rime"`) || !strings.Contains(readme, "--output-dir") {
		t.Fatal("README should document output directory configuration")
	}
	if strings.Contains(readme, "dict_path") || strings.Contains(readme, "--output ") {
		t.Fatal("README should not document old output file path configuration")
	}
	for _, stale := range []string{
		"[bootstrap.popular]",
		"popular_pages",
		"trending_week_pages",
		"top_rated_pages",
		"popular 模式",
		"series-popular.sqlite",
		"tmdb_popular_hans",
		"tmdb_popular_hant",
	} {
		if strings.Contains(readme, stale) {
			t.Fatalf("README should not document popular mode reference %q", stale)
		}
	}
	if !strings.Contains(readme, "min_popularity = 10") || !strings.Contains(readme, "popularity >= min_popularity") {
		t.Fatal("README should document full min_popularity filtering")
	}
	if !strings.Contains(readme, "movie_min_popularity = 15") || !strings.Contains(readme, "popularity >= movie_min_popularity") {
		t.Fatal("README should document full movie_min_popularity filtering")
	}
	for _, want := range []string{
		"流行度阈值",
		"不要在同一个未完成的 full bootstrap 中途修改",
		"不会自动回头补抓",
		"完成后会通过 Daily Export 本地 diff",
		"低于阈值或消失的条目会从 SQLite store 删除",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README should document popularity threshold caveat %q", want)
		}
	}
	if !strings.Contains(readme, `request_interval = "50ms"`) || !strings.Contains(readme, "`50ms` 等于 20 rps") {
		t.Fatal("README should document the default request interval")
	}
	if strings.Contains(readme, "state_path =") {
		t.Fatal("README default config should not include state_path")
	}
	if strings.Contains(readme, "[store]\npath = \"~/.local/state/rime-pinyin-tmdb-generator/series.sqlite\"") {
		t.Fatal("README default config should rely on the default store path")
	}
	for _, want := range []string{
		"SQLite store 默认路径",
		"series-full.sqlite",
		"movies-full.sqlite",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README should document default store path %q", want)
		}
	}
	for _, want := range []string{
		"tmdb_full_hans.dict.yaml",
		"tmdb_full_hant.dict.yaml",
		"tmdb_movie_hans.dict.yaml",
		"tmdb_movie_hant.dict.yaml",
		"tmdb_full_hans",
		"tmdb_movie_hans",
		"没有配置繁体语言时，不会生成 `_hant` 词典",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README should document dictionary name %q", want)
		}
	}
	for _, stale := range []string{
		"`tmdb_hans.dict.yaml`",
		"`tmdb_hant.dict.yaml`",
		"- tmdb_hans",
		"- tmdb_hant",
	} {
		if strings.Contains(readme, stale) {
			t.Fatalf("README should not document stale shared dictionary name %q", stale)
		}
	}
}
