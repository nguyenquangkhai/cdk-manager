package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Account struct {
	Profile string            `yaml:"profile"`
	Region  string            `yaml:"region"`
	Tags    []string          `yaml:"tags"`
	Context map[string]string `yaml:"context"`
}

type Group struct {
	Tags     []string `yaml:"tags"`
	Accounts []string `yaml:"accounts"`
}

type Stacks struct {
	Shared     []string            `yaml:"shared"`
	PerAccount map[string][]string `yaml:"perAccount"`
}

type Defaults struct {
	Concurrency     int    `yaml:"concurrency"`
	RequireApproval string `yaml:"requireApproval"`
}

type Config struct {
	Defaults Defaults           `yaml:"defaults"`
	Accounts map[string]Account `yaml:"accounts"`
	Groups   map[string]Group   `yaml:"groups"`
	Stacks   Stacks             `yaml:"stacks"`
}

// parse reads and unmarshals a config file without applying defaults or validating.
func parse(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &c, nil
}

// applyDefaults sets concurrency to 4 when unset and requireApproval to "never" when unset.
func applyDefaults(c *Config) {
	if c.Defaults.Concurrency == 0 {
		c.Defaults.Concurrency = 4
	}
	if c.Defaults.RequireApproval == "" {
		c.Defaults.RequireApproval = "never"
	}
}

// Load reads a single config file, applies defaults, validates, and returns it.
func Load(path string) (*Config, error) {
	c, err := parse(path)
	if err != nil {
		return nil, err
	}
	applyDefaults(c)
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Merge overlays over onto base and returns a new Config (neither mutated):
//   - Accounts/Groups: union; over's key wins on conflict.
//   - Defaults: over wins per-field when set (Concurrency != 0, RequireApproval != "").
//   - Stacks: if over provides any (Shared != nil or len(PerAccount) > 0),
//     use over.Stacks wholesale; else base.Stacks.
func Merge(base, over *Config) *Config {
	out := &Config{
		Defaults: base.Defaults,
		Accounts: map[string]Account{},
		Groups:   map[string]Group{},
		Stacks:   base.Stacks,
	}
	for k, v := range base.Accounts {
		out.Accounts[k] = v
	}
	for k, v := range base.Groups {
		out.Groups[k] = v
	}
	for k, v := range over.Accounts {
		out.Accounts[k] = v
	}
	for k, v := range over.Groups {
		out.Groups[k] = v
	}
	if over.Defaults.Concurrency != 0 {
		out.Defaults.Concurrency = over.Defaults.Concurrency
	}
	if over.Defaults.RequireApproval != "" {
		out.Defaults.RequireApproval = over.Defaults.RequireApproval
	}
	if over.Stacks.Shared != nil || len(over.Stacks.PerAccount) > 0 {
		out.Stacks = over.Stacks
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// LoadLayered parses globalPath and localPath (each optional — a missing file is
// skipped, not an error), merges local over global, applies defaults, validates
// the merged result, and returns it. At least one of the two files must exist.
func LoadLayered(globalPath, localPath string) (*Config, error) {
	var g, l *Config
	var err error
	if fileExists(globalPath) {
		if g, err = parse(globalPath); err != nil {
			return nil, err
		}
	}
	if fileExists(localPath) {
		if l, err = parse(localPath); err != nil {
			return nil, err
		}
	}
	var merged *Config
	switch {
	case g != nil && l != nil:
		merged = Merge(g, l)
	case g != nil:
		merged = g
	case l != nil:
		merged = l
	default:
		return nil, fmt.Errorf("no config found: neither %s nor %s exists", globalPath, localPath)
	}
	applyDefaults(merged)
	if err := merged.Validate(); err != nil {
		return nil, err
	}
	return merged, nil
}

// GlobalConfigPath resolves the machine-level config path:
// $CDKM_GLOBAL_CONFIG, else $XDG_CONFIG_HOME/cdkm/config.yaml,
// else ~/.config/cdkm/config.yaml.
func GlobalConfigPath() string {
	if p := os.Getenv("CDKM_GLOBAL_CONFIG"); p != "" {
		return p
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "cdkm", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "cdkm", "config.yaml")
}

func (c *Config) Validate() error {
	if len(c.Accounts) == 0 {
		return fmt.Errorf("config has no accounts")
	}
	for name := range c.Stacks.PerAccount {
		if _, ok := c.Accounts[name]; !ok {
			return fmt.Errorf("stacks.perAccount references unknown account %q", name)
		}
	}
	for gname, g := range c.Groups {
		n := 0
		for _, a := range g.Accounts {
			if _, ok := c.Accounts[a]; !ok {
				return fmt.Errorf("group %q references unknown account %q", gname, a)
			}
			n++
		}
		if len(g.Tags) > 0 {
			for _, acc := range c.Accounts {
				if hasAnyTag(acc.Tags, g.Tags) {
					n++
				}
			}
		}
		if n == 0 {
			return fmt.Errorf("group %q resolves to zero accounts", gname)
		}
	}
	return nil
}

func hasAnyTag(have, want []string) bool {
	set := make(map[string]struct{}, len(have))
	for _, t := range have {
		set[t] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; ok {
			return true
		}
	}
	return false
}
