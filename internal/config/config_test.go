package config

import "testing"

func TestLoadValid(t *testing.T) {
	c, err := Load("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Defaults.Concurrency != 4 {
		t.Errorf("concurrency = %d, want 4 (default applied)", c.Defaults.Concurrency)
	}
	if _, ok := c.Accounts["dev-eu"]; !ok {
		t.Errorf("missing account dev-eu")
	}
	if got := c.Accounts["dev-eu"].Context["env"]; got != "dev" {
		t.Errorf("dev-eu context env = %q, want dev", got)
	}
}

func TestValidateUnknownPerAccount(t *testing.T) {
	c := &Config{
		Accounts: map[string]Account{"a": {Profile: "a", Region: "r"}},
		Stacks:   Stacks{PerAccount: map[string][]string{"ghost": {"S"}}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for unknown perAccount key, got nil")
	}
}

func TestValidateEmptyGroup(t *testing.T) {
	c := &Config{
		Accounts: map[string]Account{"a": {Profile: "a", Region: "r", Tags: []string{"dev"}}},
		Groups:   map[string]Group{"prod": {Tags: []string{"prod"}}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for group resolving to zero accounts, got nil")
	}
}
