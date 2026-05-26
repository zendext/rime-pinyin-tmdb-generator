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

func TestRunStatusReportsUnknownBootstrapModeNotTimerReady(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	storePath := filepath.Join(dir, "series.sqlite")
	configData := []byte("[tmdb]\napi_key = \"test\"\n[store]\npath = \"" + storePath + "\"\n[bootstrap]\nmode = \"legacy\"\n")
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "--config", configPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "timer_ready=false") {
		t.Fatalf("expected timer_ready=false in status output:\n%s", stdout.String())
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

func TestDocsShowSingleDefaultLanguageAndPopularStatus(t *testing.T) {
	configData, err := os.ReadFile("../../examples/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	configText := string(configData)
	if !strings.Contains(configText, "languages = [\"zh-CN\"]") {
		t.Fatalf("example config should default to one language:\n%s", configText)
	}
	if !strings.Contains(configText, "max_pages = 10") {
		t.Fatalf("example config should default max_pages to 10:\n%s", configText)
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

	readmeData, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(readmeData)
	if !strings.Contains(readme, "languages = [\"zh-CN\"]") {
		t.Fatal("README should default to one language")
	}
	if !strings.Contains(readme, "max_pages = 10") {
		t.Fatal("README should default max_pages to 10")
	}
	if !strings.Contains(readme, "popular 和 full 模式都可以用 `status`") {
		t.Fatal("README should explain status works for popular and full modes")
	}
	if strings.Contains(readme, "state_path =") {
		t.Fatal("README default config should not include state_path")
	}
	if strings.Contains(readme, "[store]\npath = \"~/.local/state/rime-pinyin-tmdb-generator/series.sqlite\"") {
		t.Fatal("README default config should rely on the default store path")
	}
	if !strings.Contains(readme, "SQLite store 默认写到") {
		t.Fatal("README should document the default store path")
	}
}
