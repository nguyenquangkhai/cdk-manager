package config

import (
	"fmt"
	"os"

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

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if c.Defaults.Concurrency == 0 {
		c.Defaults.Concurrency = 4
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
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
