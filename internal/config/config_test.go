package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValid(t *testing.T) {
	c, err := Load("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Defaults.Concurrency != 4 {
		t.Errorf("concurrency = %d, want 4 (default applied)", c.Defaults.Concurrency)
	}
	if c.Defaults.RequireApproval != "never" {
		t.Errorf("requireApproval = %q, want never", c.Defaults.RequireApproval)
	}
	if _, ok := c.Accounts["dev-eu"]; !ok {
		t.Errorf("missing account dev-eu")
	}
	if got := c.Accounts["dev-eu"].Context["env"]; got != "dev" {
		t.Errorf("dev-eu context env = %q, want dev", got)
	}
}

func TestLoadDefaultsRequireApproval(t *testing.T) {
	c, err := Load("testdata/no-approval.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Defaults.RequireApproval != "never" {
		t.Errorf("requireApproval = %q, want never (default applied)", c.Defaults.RequireApproval)
	}
	if c.Defaults.Concurrency != 4 {
		t.Errorf("concurrency = %d, want 4 (default applied)", c.Defaults.Concurrency)
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

func TestMergeLocalWins(t *testing.T) {
	global := &Config{
		Defaults: Defaults{Concurrency: 8, RequireApproval: "broadening"},
		Accounts: map[string]Account{
			"prod-eu": {Profile: "prod-eu", Region: "eu-west-1", Tags: []string{"prod"}},
			"prod-us": {Profile: "prod-us", Region: "us-east-1", Tags: []string{"prod"}},
		},
		Groups: map[string]Group{"prod": {Tags: []string{"prod"}}},
	}
	local := &Config{
		Defaults: Defaults{Concurrency: 2}, // overrides concurrency, keeps global requireApproval
		Accounts: map[string]Account{
			"prod-eu": {Profile: "prod-eu-override", Region: "eu-west-1"}, // override one
		},
		Stacks: Stacks{Shared: []string{"AppStack"}},
	}
	m := Merge(global, local)
	if m.Defaults.Concurrency != 2 {
		t.Errorf("concurrency = %d, want 2 (local wins)", m.Defaults.Concurrency)
	}
	if m.Defaults.RequireApproval != "broadening" {
		t.Errorf("requireApproval = %q, want broadening (global kept)", m.Defaults.RequireApproval)
	}
	if m.Accounts["prod-eu"].Profile != "prod-eu-override" {
		t.Errorf("prod-eu profile = %q, want override", m.Accounts["prod-eu"].Profile)
	}
	if _, ok := m.Accounts["prod-us"]; !ok {
		t.Error("prod-us from global should survive merge")
	}
	if len(m.Stacks.Shared) != 1 || m.Stacks.Shared[0] != "AppStack" {
		t.Errorf("stacks = %v, want [AppStack] from local", m.Stacks.Shared)
	}
	// merging must not mutate inputs
	if global.Accounts["prod-eu"].Profile != "prod-eu" {
		t.Error("Merge mutated the global input")
	}
}

func TestLoadLayeredGlobalPlusLocal(t *testing.T) {
	dir := t.TempDir()
	g := filepath.Join(dir, "global.yaml")
	l := filepath.Join(dir, "cdkm.yaml")
	if err := os.WriteFile(g, []byte(`
accounts:
  prod-eu: { profile: prod-eu, region: eu-west-1, tags: [prod] }
groups:
  prod: { tags: [prod] }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l, []byte(`
stacks:
  shared: [AppStack]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadLayered(g, l)
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	if _, ok := c.Accounts["prod-eu"]; !ok {
		t.Error("account from global missing")
	}
	if c.Defaults.Concurrency != 4 {
		t.Errorf("concurrency default not applied: %d", c.Defaults.Concurrency)
	}
	if len(c.Stacks.Shared) != 1 {
		t.Errorf("local stacks not merged: %v", c.Stacks)
	}
}

func TestLoadLayeredOnlyGlobal(t *testing.T) {
	dir := t.TempDir()
	g := filepath.Join(dir, "global.yaml")
	if err := os.WriteFile(g, []byte("accounts:\n  a: { profile: a, region: r }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadLayered(g, filepath.Join(dir, "missing-local.yaml"))
	if err != nil {
		t.Fatalf("only-global should load: %v", err)
	}
	if _, ok := c.Accounts["a"]; !ok {
		t.Error("account a missing")
	}
}

func TestLoadLayeredNeitherErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadLayered(filepath.Join(dir, "no-g"), filepath.Join(dir, "no-l")); err == nil {
		t.Fatal("expected error when neither file exists")
	}
}
