package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOverridesAcceptsSimpleAndObjectEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	data := []byte("entries:\n  长安剧场: chang an ju chang\n  虚构剧集:\n    pinyin: xu gou ju ji\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadOverrides(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["长安剧场"] != "chang an ju chang" || got["虚构剧集"] != "xu gou ju ji" {
		t.Fatalf("unexpected overrides: %#v", got)
	}
}

func TestLoadExpandsTildePaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := []byte("[output]\ndir = \"~/rime-data\"\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(got.Output.Dir, "~") {
		t.Fatalf("expected tilde expansion, got %q", got.Output.Dir)
	}
}

func TestLoadUsesTMDBConfigAndEnvironment(t *testing.T) {
	t.Setenv("TMDB_API_KEY", "env-key")
	path := filepath.Join(t.TempDir(), "config.toml")
	data := []byte("[tmdb]\napi_key = \"file-key\"\nbase_url = \"https://example.test/3\"\nlanguages = [\"zh-CN\"]\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if got.TMDB.APIKey != "env-key" {
		t.Fatalf("expected TMDB_API_KEY to override config value, got %q", got.TMDB.APIKey)
	}
	if got.TMDB.BaseURL != "https://example.test/3" {
		t.Fatalf("unexpected base URL: %q", got.TMDB.BaseURL)
	}
	if len(got.TMDB.Languages) != 1 || got.TMDB.Languages[0] != "zh-CN" {
		t.Fatalf("unexpected languages: %#v", got.TMDB.Languages)
	}
}

func TestLoadRejectsPopularBootstrapMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := []byte("[bootstrap]\nmode = \"popular\"\n[bootstrap.popular]\ntrending_week_pages = 2\npopular_pages = 4\ntop_rated_pages = 6\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || err.Error() != `unsupported bootstrap mode "popular"` {
		t.Fatalf("expected popular mode rejection, got %v", err)
	}
}

func TestLoadUsesFullDefaultStorePath(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)

	fullConfigPath := filepath.Join(t.TempDir(), "full.toml")
	if err := os.WriteFile(fullConfigPath, []byte("[bootstrap]\nmode = \"full\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	full, err := Load(fullConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	fullWant := filepath.Join(stateDir, "rime-pinyin-tmdb-generator", "series-full.sqlite")
	if full.Store.Path != fullWant {
		t.Fatalf("expected full default store %q, got %q", fullWant, full.Store.Path)
	}
}

func TestApplyModeDefaultsUsesFinalModeUnlessStorePathIsExplicit(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)

	cfg := Default()
	cfg.Bootstrap.Mode = "full"
	ApplyModeDefaults(&cfg)
	fullWant := filepath.Join(stateDir, "rime-pinyin-tmdb-generator", "series-full.sqlite")
	if cfg.Store.Path != fullWant {
		t.Fatalf("expected final full mode store %q, got %q", fullWant, cfg.Store.Path)
	}

	cfg.Store.Path = filepath.Join(stateDir, "custom.sqlite")
	cfg.Store.PathExplicit = true
	cfg.Bootstrap.Mode = "legacy"
	ApplyModeDefaults(&cfg)
	if cfg.Store.Path != filepath.Join(stateDir, "custom.sqlite") {
		t.Fatalf("expected explicit store path to be preserved, got %q", cfg.Store.Path)
	}
}

func TestLoadUsesFullBootstrapAndStoreConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := []byte("[store]\npath = \"~/state/series.sqlite\"\nmovie_path = \"~/state/movies.sqlite\"\n[bootstrap]\nmode = \"full\"\nexport_date = \"05_26_2026\"\nrequest_interval = \"200ms\"\nmax_items = 100\nmin_popularity = 12.5\nmovie_min_popularity = 17.5\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bootstrap.Mode != "full" {
		t.Fatalf("unexpected bootstrap mode: %q", got.Bootstrap.Mode)
	}
	if got.Bootstrap.ExportDate != "05_26_2026" {
		t.Fatalf("unexpected export date: %q", got.Bootstrap.ExportDate)
	}
	if got.Bootstrap.RequestInterval.Std().String() != "200ms" {
		t.Fatalf("unexpected request interval: %s", got.Bootstrap.RequestInterval.Std())
	}
	if got.Bootstrap.MaxItems != 100 {
		t.Fatalf("unexpected max items: %d", got.Bootstrap.MaxItems)
	}
	if got.Bootstrap.MinPopularity != 12.5 {
		t.Fatalf("unexpected min popularity: %v", got.Bootstrap.MinPopularity)
	}
	if got.Bootstrap.MovieMinPopularity != 17.5 {
		t.Fatalf("unexpected movie min popularity: %v", got.Bootstrap.MovieMinPopularity)
	}
	if strings.HasPrefix(got.Store.Path, "~") {
		t.Fatalf("expected store path expansion, got %q", got.Store.Path)
	}
	if strings.HasPrefix(got.Store.MoviePath, "~") {
		t.Fatalf("expected movie store path expansion, got %q", got.Store.MoviePath)
	}
}

func TestDefaultFullBootstrapPopularityThresholdsAndStores(t *testing.T) {
	got := Default()
	if got.Bootstrap.Mode != "full" {
		t.Fatalf("expected default bootstrap mode full, got %q", got.Bootstrap.Mode)
	}
	if got.Bootstrap.RequestInterval.Std().String() != "50ms" {
		t.Fatalf("expected default request interval 50ms, got %s", got.Bootstrap.RequestInterval.Std())
	}
	if got.Bootstrap.MinPopularity != 10 {
		t.Fatalf("expected default min popularity 10, got %v", got.Bootstrap.MinPopularity)
	}
	if got.Bootstrap.MovieMinPopularity != 15 {
		t.Fatalf("expected default movie min popularity 15, got %v", got.Bootstrap.MovieMinPopularity)
	}
	if !strings.HasSuffix(got.Store.Path, filepath.Join("rime-pinyin-tmdb-generator", "series-full.sqlite")) {
		t.Fatalf("expected default store path to use full mode, got %q", got.Store.Path)
	}
	if !strings.HasSuffix(got.Store.MoviePath, filepath.Join("rime-pinyin-tmdb-generator", "movies-full.sqlite")) {
		t.Fatalf("expected default movie store path to use full mode, got %q", got.Store.MoviePath)
	}
}

func TestDefaultOutputDirUsesFcitx5RimeUserDirWhenPresent(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataDir)
	rimeDir := filepath.Join(dataDir, "fcitx5", "rime")
	if err := os.MkdirAll(rimeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := Default()
	if got.Output.Dir != rimeDir {
		t.Fatalf("expected default output dir %q, got %q", rimeDir, got.Output.Dir)
	}
}

func TestDefaultOutputDirFallsBackToRimeData(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataDir)

	got := Default()
	want := filepath.Join(dataDir, "rime-data")
	if got.Output.Dir != want {
		t.Fatalf("expected default output dir %q, got %q", want, got.Output.Dir)
	}
}
