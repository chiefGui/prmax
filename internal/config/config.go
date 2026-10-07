package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Repo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Config struct {
	Listen       string `json:"listen"`
	Concurrency  int    `json:"concurrency"`
	ClaudePath   string `json:"claudePath"`
	GhPath       string `json:"ghPath"`
	CodexPath    string `json:"codexPath"`
	Instructions string `json:"instructions"`
	AppID        string `json:"appId"`
	AppKey       string `json:"appKey"`
	Repos        []Repo `json:"repos"`
}

func Root() string {
	if d := os.Getenv("PRMAX_HOME"); d != "" {
		return d
	}
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Dir(filepath.Dir(exe))
}

func Dir() string { return filepath.Join(Root(), ".prmax") }

func Path() string { return filepath.Join(Root(), "config.json") }

func Default() Config {
	return Config{
		Listen:      "127.0.0.1:47821",
		Concurrency: 2,
		ClaudePath:  "claude",
		GhPath:      "gh",
		CodexPath:   "codex",
	}
}

func Load() (Config, error) {
	c := Default()
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		if err := Save(c); err != nil {
			return c, err
		}
		return c, fmt.Errorf("created %s; add your repos there", Path())
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", Path(), err)
	}
	if c.Concurrency < 1 {
		c.Concurrency = 1
	}
	return c, nil
}

func Save(c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), b, 0o644)
}

func (c Config) Repo(name string) (Repo, bool) {
	for _, r := range c.Repos {
		if r.Name == name {
			return r, true
		}
	}
	return Repo{}, false
}

func (c Config) AppKeyPath() string {
	if c.AppKey == "" || filepath.IsAbs(c.AppKey) {
		return c.AppKey
	}
	return filepath.Join(Root(), c.AppKey)
}
