package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"prmax/internal/config"
	"prmax/internal/model"
)

var claudeEfforts = []string{"low", "medium", "high", "xhigh", "max"}

var claudeModels = []model.ModelOption{
	{ID: "opus", Label: "Opus 5.5", Efforts: claudeEfforts, DefaultEffort: "high"},
	{ID: "fable", Label: "Fable 5.1", Efforts: claudeEfforts, DefaultEffort: "high"},
	{ID: "sonnet", Label: "Sonnet 5.5", Efforts: claudeEfforts, DefaultEffort: "high"},
	{ID: "haiku", Label: "Haiku 4.5", Efforts: claudeEfforts, DefaultEffort: "high"},
}

var choiceMu sync.Mutex

func choicePath() string { return filepath.Join(config.Dir(), "choice.json") }

func loadChoice() model.Choice {
	var c model.Choice
	if b, err := os.ReadFile(choicePath()); err == nil {
		json.Unmarshal(b, &c)
	}
	if c.Models == nil {
		c.Models = map[string]string{}
	}
	if c.Efforts == nil {
		c.Efforts = map[string]string{}
	}
	return c
}

func saveChoice(req model.ReviewRequest) {
	choiceMu.Lock()
	defer choiceMu.Unlock()
	c := loadChoice()
	c.Provider = req.Provider
	c.Models[req.Provider] = req.Model
	if req.Effort != "" {
		c.Efforts[req.Provider] = req.Effort
	}
	c.Access = req.Access
	c.Scope = req.Scope
	c.Nudge = req.Nudge
	b, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(choicePath(), b, 0o644)
}

func (d *Daemon) providers() model.Providers {
	codex := codexModels()
	def := codexDefault()
	if def == "" && len(codex) > 0 {
		def = codex[0].ID
	}
	out := model.Providers{
		List: []model.Provider{
			{ID: "claude", Label: "Claude", Models: claudeModels, Default: "opus"},
			{ID: "codex", Label: "Codex", Models: codex, Default: def},
		},
		Choice: loadChoice(),
	}
	if out.Choice.Access == "" {
		out.Choice.Access = model.AccessFull
	}
	if out.Choice.Scope == "" {
		out.Choice.Scope = model.ScopeChanges
	}
	if out.Choice.Provider == "" {
		out.Choice.Provider = "claude"
	}
	return out
}

func codexHome() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

func codexModels() []model.ModelOption {
	b, err := os.ReadFile(filepath.Join(codexHome(), "models_cache.json"))
	if err != nil {
		return nil
	}
	var cache struct {
		Models []struct {
			Slug          string `json:"slug"`
			DisplayName   string `json:"display_name"`
			Visibility    string `json:"visibility"`
			DefaultEffort string `json:"default_reasoning_level"`
			Levels        []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if json.Unmarshal(b, &cache) != nil {
		return nil
	}
	var out []model.ModelOption
	for _, m := range cache.Models {
		if m.Visibility != "list" {
			continue
		}
		label := m.DisplayName
		if label == "" {
			label = m.Slug
		}
		opt := model.ModelOption{ID: m.Slug, Label: label, DefaultEffort: m.DefaultEffort}
		for _, l := range m.Levels {
			opt.Efforts = append(opt.Efforts, l.Effort)
		}
		if e := codexDefaultEffort(); e != "" && contains(opt.Efforts, e) {
			opt.DefaultEffort = e
		}
		out = append(out, opt)
	}
	return out
}

var codexModelLine = regexp.MustCompile(`(?m)^\s*model\s*=\s*"([^"]+)"`)

var codexEffortLine = regexp.MustCompile(`(?m)^\s*model_reasoning_effort\s*=\s*"([^"]+)"`)

func codexDefaultEffort() string {
	b, err := os.ReadFile(filepath.Join(codexHome(), "config.toml"))
	if err != nil {
		return ""
	}
	if m := codexEffortLine.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func codexDefault() string {
	b, err := os.ReadFile(filepath.Join(codexHome(), "config.toml"))
	if err != nil {
		return ""
	}
	if m := codexModelLine.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}

func (d *Daemon) label(provider, id, effort string) string {
	name := id
	for _, p := range d.providers().List {
		if p.ID != provider {
			continue
		}
		for _, m := range p.Models {
			if m.ID == id {
				name = m.Label
			}
		}
	}
	if effort != "" {
		name += " · " + effort
	}
	return name
}
