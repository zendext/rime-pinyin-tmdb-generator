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
	DictPath string `toml:"dict_path"`
	LockPath string `toml:"lock_path"`
}

type StoreConfig struct {
	Path string `toml:"path"`
}

type BootstrapConfig struct {
	Mode            string        `toml:"mode"`
	Popular         PopularConfig `toml:"popular"`
	ExportDate      string        `toml:"export_date"`
	ExportBaseURL   string        `toml:"export_base_url"`
	RequestInterval Duration      `toml:"request_interval"`
	MaxItems        int           `toml:"max_items"`
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
			DictPath: filepath.Join(dataDir, "rime-data", "tmdb.dict.yaml"),
			LockPath: filepath.Join(baseState, "update.lock"),
		},
		Store: StoreConfig{
			Path: filepath.Join(baseState, "series.sqlite"),
		},
		Bootstrap: BootstrapConfig{
			Mode:            "popular",
			Popular:         PopularConfig{TrendingWeekPages: 5, PopularPages: 10, TopRatedPages: 10},
			RequestInterval: Duration(200 * time.Millisecond),
		},
		Overrides: filepath.Join(baseConfig, "overrides.yaml"),
	}
}

func DefaultPath() string {
	return filepath.Join(xdg("XDG_CONFIG_HOME", filepath.Join(homeDir(), ".config")), "rime-pinyin-tmdb-generator", "config.toml")
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = DefaultPath()
	}
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, &cfg); err != nil {
			return Config{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	applyEnv(&cfg)
	expandPaths(&cfg)
	return cfg, nil
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
	cfg.Output.DictPath = expandHome(cfg.Output.DictPath)
	cfg.Output.LockPath = expandHome(cfg.Output.LockPath)
	cfg.Store.Path = expandHome(cfg.Store.Path)
	cfg.Overrides = expandHome(cfg.Overrides)
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
