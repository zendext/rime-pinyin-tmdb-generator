package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type Config struct {
	TMDB      TMDBConfig      `toml:"tmdb"`
	Output    OutputConfig    `toml:"output"`
	Store     StoreConfig     `toml:"store"`
	Bootstrap BootstrapConfig `toml:"bootstrap"`
	Rime      RimeConfig      `toml:"rime"`
	Overrides string          `toml:"overrides"`
}

type TMDBConfig struct {
	APIKey    string   `toml:"api_key"`
	BaseURL   string   `toml:"base_url"`
	Languages []string `toml:"languages"`
}

type OutputConfig struct {
	Dir      string `toml:"dir"`
	LockPath string `toml:"lock_path"`
}

type StoreConfig struct {
	Path              string `toml:"path"`
	MoviePath         string `toml:"movie_path"`
	PathExplicit      bool   `toml:"-"`
	MoviePathExplicit bool   `toml:"-"`
}

type BootstrapConfig struct {
	Mode               string        `toml:"mode"`
	Popular            PopularConfig `toml:"popular"`
	ExportDate         string        `toml:"export_date"`
	ExportBaseURL      string        `toml:"export_base_url"`
	RequestInterval    Duration      `toml:"request_interval"`
	MaxItems           int           `toml:"max_items"`
	MinPopularity      float64       `toml:"min_popularity"`
	MovieMinPopularity float64       `toml:"movie_min_popularity"`
}

type PopularConfig struct {
	TrendingWeekPages int `toml:"trending_week_pages"`
	PopularPages      int `toml:"popular_pages"`
	TopRatedPages     int `toml:"top_rated_pages"`
}

type RimeConfig struct {
	RedeployCommand string `toml:"redeploy_command"`
}

func Default() Config {
	stateDir := xdg("XDG_STATE_HOME", filepath.Join(homeDir(), ".local", "state"))
	configDir := xdg("XDG_CONFIG_HOME", filepath.Join(homeDir(), ".config"))
	dataDir := xdg("XDG_DATA_HOME", filepath.Join(homeDir(), ".local", "share"))
	baseState := filepath.Join(stateDir, "rime-pinyin-tmdb-generator")
	baseConfig := filepath.Join(configDir, "rime-pinyin-tmdb-generator")
	return Config{
		TMDB: TMDBConfig{
			BaseURL:   "https://api.themoviedb.org/3",
			Languages: []string{"zh-CN", "zh-TW", "zh-HK"},
		},
		Output: OutputConfig{
			Dir:      defaultOutputDir(dataDir),
			LockPath: filepath.Join(baseState, "update.lock"),
		},
		Store: StoreConfig{
			Path:      filepath.Join(baseState, "series-full.sqlite"),
			MoviePath: filepath.Join(baseState, "movies-full.sqlite"),
		},
		Bootstrap: BootstrapConfig{
			Mode:               "full",
			Popular:            PopularConfig{TrendingWeekPages: 5, PopularPages: 10, TopRatedPages: 10},
			RequestInterval:    Duration(50 * time.Millisecond),
			MinPopularity:      10,
			MovieMinPopularity: 15,
		},
		Overrides: filepath.Join(baseConfig, "overrides.yaml"),
	}
}

func DefaultPath() string {
	return filepath.Join(xdg("XDG_CONFIG_HOME", filepath.Join(homeDir(), ".config")), "rime-pinyin-tmdb-generator", "config.toml")
}

func Load(path string) (Config, error) {
	cfg := Default()
	storePathExplicit := false
	movieStorePathExplicit := false
	if path == "" {
		path = DefaultPath()
	}
	if _, err := os.Stat(path); err == nil {
		meta, err := toml.DecodeFile(path, &cfg)
		if err != nil {
			return Config{}, err
		}
		storePathExplicit = meta.IsDefined("store", "path")
		movieStorePathExplicit = meta.IsDefined("store", "movie_path")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	cfg.Store.PathExplicit = storePathExplicit
	cfg.Store.MoviePathExplicit = movieStorePathExplicit
	applyEnv(&cfg)
	ApplyModeDefaults(&cfg)
	expandPaths(&cfg)
	return cfg, nil
}

func ApplyModeDefaults(cfg *Config) {
	if !cfg.Store.PathExplicit {
		cfg.Store.Path = defaultStorePath(cfg.Bootstrap.Mode)
	}
	if !cfg.Store.MoviePathExplicit {
		cfg.Store.MoviePath = defaultMovieStorePath()
	}
}

func LoadOverrides(path string) (map[string]string, error) {
	out := make(map[string]string)
	if path == "" {
		return out, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	var file struct {
		Entries map[string]Override `yaml:"entries"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	for word, entry := range file.Entries {
		if p := strings.TrimSpace(entry.Pinyin); strings.TrimSpace(word) != "" && p != "" {
			out[strings.TrimSpace(word)] = p
		}
	}
	return out, nil
}

type Override struct {
	Pinyin string
}

func (o *Override) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		o.Pinyin = value.Value
		return nil
	}
	var obj struct {
		Pinyin string `yaml:"pinyin"`
	}
	if err := value.Decode(&obj); err != nil {
		return err
	}
	o.Pinyin = obj.Pinyin
	return nil
}

func applyEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("TMDB_API_KEY")); v != "" {
		cfg.TMDB.APIKey = v
	}
}

func expandPaths(cfg *Config) {
	cfg.Output.Dir = expandHome(cfg.Output.Dir)
	cfg.Output.LockPath = expandHome(cfg.Output.LockPath)
	cfg.Store.Path = expandHome(cfg.Store.Path)
	cfg.Store.MoviePath = expandHome(cfg.Store.MoviePath)
	cfg.Overrides = expandHome(cfg.Overrides)
}

func defaultStorePath(mode string) string {
	stateDir := xdg("XDG_STATE_HOME", filepath.Join(homeDir(), ".local", "state"))
	baseState := filepath.Join(stateDir, "rime-pinyin-tmdb-generator")
	mode = strings.TrimSpace(mode)
	if mode == "full" {
		return filepath.Join(baseState, "series-full.sqlite")
	}
	return filepath.Join(baseState, "series-popular.sqlite")
}

func defaultOutputDir(dataDir string) string {
	fcitx5RimeDir := filepath.Join(dataDir, "fcitx5", "rime")
	if info, err := os.Stat(fcitx5RimeDir); err == nil && info.IsDir() {
		return fcitx5RimeDir
	}
	return filepath.Join(dataDir, "rime-data")
}

func defaultMovieStorePath() string {
	stateDir := xdg("XDG_STATE_HOME", filepath.Join(homeDir(), ".local", "state"))
	baseState := filepath.Join(stateDir, "rime-pinyin-tmdb-generator")
	return filepath.Join(baseState, "movies-full.sqlite")
}

type Duration time.Duration

func (d *Duration) UnmarshalText(text []byte) error {
	value, err := time.ParseDuration(strings.TrimSpace(string(text)))
	if err != nil {
		return err
	}
	*d = Duration(value)
	return nil
}

func (d Duration) Std() time.Duration {
	return time.Duration(d)
}

func expandHome(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir(), strings.TrimPrefix(path, "~/"))
	}
	return path
}

func xdg(env, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(env)); value != "" {
		return value
	}
	return fallback
}

func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "."
}
