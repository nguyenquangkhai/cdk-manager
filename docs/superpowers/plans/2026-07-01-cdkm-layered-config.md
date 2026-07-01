# cdkm — layered global/local config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Let one machine-level config define accounts/groups once, so each CDK project only needs its `stacks:` (or nothing). `cdkm` merges a global `~/.config/cdkm/config.yaml` with the local `./cdkm.yaml`; `cdkm init --global` writes the shared accounts config.

**Architecture:** Split config parsing from validation so partial files (global = no stacks; local = no accounts) can be parsed then merged and validated as a whole. A pure `Merge(base, over)` overlays local onto global (local wins). `LoadLayered(globalPath, localPath)` parses whichever exist, merges, defaults, validates. The CLI resolves the global path and calls `LoadLayered`. `cdkm init --global` routes generated accounts/groups to the global path.

**Tech Stack:** Go 1.24, existing `internal/config`, `internal/awsconfig`, cobra.

## Global Constraints

- Module path: `github.com/nguyenquangkhai/cdk-manager` (verbatim in imports).
- Go floor stays **1.24**.
- Global config path resolution order: `$CDKM_GLOBAL_CONFIG` → `$XDG_CONFIG_HOME/cdkm/config.yaml` → `~/.config/cdkm/config.yaml`.
- **Backward compatibility is mandatory:** existing `config.Load(path)` behavior and its tests must not change. Only-local usage must behave exactly as before. Only-global (no local `cdkm.yaml`) must work. Neither present → error.
- Merge precedence: local overrides global (accounts/groups by key; defaults field-by-field when set; stacks replaced wholesale if local provides any).
- CI lint is strict + green: before each commit run `gofmt -l .` (empty), `go vet ./...`, `go test ./...`, `go run honnef.co/go/tools/cmd/staticcheck@2025.1 ./...` (clean, no unused symbols).
- Commit after each task; Conventional Commits. Body ends with:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`

---

### Task 1: layered config loading (`parse` + `Merge` + `LoadLayered` + global path)

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go` (add cases; do not alter existing ones)
- Modify: `cmd/cdkm/commands.go` (and/or wherever `config.Load` is called) to use `LoadLayered`

**Interfaces:**
- Produces (in `internal/config`):
  ```go
  // parse reads+unmarshals a config file WITHOUT applying defaults or
  // validating (so partial global/local files can be merged first).
  func parse(path string) (*Config, error)

  // Merge overlays over onto base and returns a new Config (neither mutated):
  //  - Accounts/Groups: union; over's key wins on conflict.
  //  - Defaults: over wins per-field when set (Concurrency != 0, RequireApproval != "").
  //  - Stacks: if over provides any (Shared != nil or len(PerAccount) > 0),
  //    use over.Stacks wholesale; else base.Stacks.
  func Merge(base, over *Config) *Config

  // LoadLayered parses globalPath and localPath (each optional — a missing
  // file is skipped, not an error), merges local over global, applies the
  // concurrency default, validates the merged result, and returns it.
  // At least one of the two files must exist.
  func LoadLayered(globalPath, localPath string) (*Config, error)

  // GlobalConfigPath resolves the machine-level config path:
  // $CDKM_GLOBAL_CONFIG, else $XDG_CONFIG_HOME/cdkm/config.yaml,
  // else ~/.config/cdkm/config.yaml.
  func GlobalConfigPath() string
  ```
- `Load(path)` keeps its exact current behavior (refactor it to `parse` + default + `Validate`, producing identical results).

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go` (keep existing tests intact):

```go
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
	os.WriteFile(g, []byte("accounts:\n  a: { profile: a, region: r }\n"), 0o644)
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
```

(Ensure `os` and `path/filepath` are imported in the test file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: FAIL — undefined `Merge`/`LoadLayered`.

- [ ] **Step 3: Implement**

In `internal/config/config.go`:
- Extract the read+unmarshal portion of `Load` into `parse(path)` (no default, no validate). Reimplement `Load` as: `c, err := parse(path); if err…; applyDefaults(c); if err := c.Validate()…; return c`. Keep behavior identical.
- Add `applyDefaults(c *Config)` helper setting `Concurrency = 4` when 0 (extracted from current Load).
- Add `Merge`, `LoadLayered`, `GlobalConfigPath`:

```go
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

func applyDefaults(c *Config) {
	if c.Defaults.Concurrency == 0 {
		c.Defaults.Concurrency = 4
	}
}

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
```

Add `"path/filepath"` to config.go imports.

- [ ] **Step 4: Wire the CLI to LoadLayered**

In `cmd/cdkm/commands.go` (and any other `config.Load` call site — `list`/`status`/`run`/the cdk ops), replace `config.Load(configPath)` with `config.LoadLayered(config.GlobalConfigPath(), configPath)`. The `--config` flag still names the LOCAL file (default `cdkm.yaml`). Grep for `config.Load(` to catch every call site.

- [ ] **Step 5: Run tests + full verify**

Run: `go test ./... && go build ./... && go vet ./... && gofmt -l . | grep -v testdata`
Expected: tests PASS (existing config tests unchanged + new ones); no gofmt output. Run staticcheck clean.

- [ ] **Step 6: Commit**

```bash
git add internal/config cmd/cdkm
git commit -m "feat: layered global+local config (LoadLayered/Merge/GlobalConfigPath)"
```

---

### Task 2: `cdkm init --global` + docs

**Files:**
- Modify: `cmd/cdkm/init.go`
- Test: `cmd/cdkm/init_test.go`
- Modify: `README.md`, `CHANGELOG.md`

**Interfaces:**
- Produces:
  ```go
  // stripStacks returns generated YAML with the stacks section removed, for
  // the global accounts-only config. (Global files describe accounts/groups;
  // stacks are project-level.)
  func generateGlobal(sels []awsconfig.Selection) ([]byte, error)
  ```
  Implementation: reuse `awsconfig.Generate` but omit stacks — simplest is a dedicated marshal of just accounts+groups. Put `generateGlobal` in `cmd/cdkm/init.go` (it can marshal a struct with only accounts+groups) OR add an `awsconfig.GenerateAccounts(sels)` in the awsconfig package. Prefer `awsconfig.GenerateAccounts` for symmetry with `Generate`; add a matching test there.

- [ ] **Step 1: Write the failing test**

Add to `internal/awsconfig` (new test) — `GenerateAccounts` omits stacks:

```go
func TestGenerateAccountsOmitsStacks(t *testing.T) {
	out, err := GenerateAccounts([]Selection{
		{Name: "a", Profile: "a", Region: "r", Tags: []string{"prod"}, Groups: []string{"prod"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "stacks") {
		t.Errorf("global config must not contain stacks:\n%s", s)
	}
	if !strings.Contains(s, "profile: a") {
		t.Errorf("missing account:\n%s", s)
	}
	var probe struct {
		Accounts map[string]map[string]any `yaml:"accounts"`
		Groups   map[string]map[string]any `yaml:"groups"`
	}
	if err := yaml.Unmarshal(out, &probe); err != nil {
		t.Fatalf("invalid yaml: %v", err)
	}
	if _, ok := probe.Accounts["a"]; !ok {
		t.Error("account a missing")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/awsconfig/ -run TestGenerateAccountsOmitsStacks`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `GenerateAccounts` + `--global`**

In `internal/awsconfig`, add `GenerateAccounts(sels)` — identical to `Generate` but the marshaled doc has only `accounts` and `groups` (no `stacks` field). (Factor the shared account/group building if convenient; keep `Generate` behavior unchanged.)

In `cmd/cdkm/init.go`, add a `--global` bool flag. When set:
- The selection flow is unchanged (checkbox / `--edit` / `--non-interactive` all still produce Selections).
- Output uses `awsconfig.GenerateAccounts(sels)` (no stacks).
- The output target is `config.GlobalConfigPath()` (create parent dir via `os.MkdirAll`), unless `--stdout`. `--force` still guards overwrite. Print where it wrote.
- `--edit` + `--global`: prefilled doc is the accounts-only doc; on save validate it parses (accounts/groups) — reuse `validateConfigBytes` (it validates the config schema; an accounts-only doc with ≥1 account passes since stacks is optional).

- [ ] **Step 4: Run tests + verify**

Run: `go test ./... && go build ./... && go vet ./... && gofmt -l . | grep -v testdata`
Expected: PASS; no gofmt output; staticcheck clean.

- [ ] **Step 5: Docs**

README: add a "Multiple projects" subsection — `cdkm init --global` writes accounts/groups to `~/.config/cdkm/config.yaml` once; each project's `cdkm.yaml` then only needs `stacks:` (or can be omitted to deploy all stacks). Note the resolution order and that local overrides global. CHANGELOG `## [Unreleased]` → Added: layered global/local config; `cdkm init --global`.

- [ ] **Step 6: Smoke + Commit + Push**

Smoke (fixtures, non-interactive, stdout so no files written):
```bash
AWS_CONFIG_FILE=internal/awsconfig/testdata/config.ini AWS_SHARED_CREDENTIALS_FILE=internal/awsconfig/testdata/credentials.ini \
  go run ./cmd/cdkm init --global --non-interactive --stdout
```
Expected: accounts+groups YAML with NO `stacks:` section.

```bash
git add cmd/cdkm internal/awsconfig README.md CHANGELOG.md
git commit -m "feat: cdkm init --global writes shared accounts config"
git push origin main
```

---

## Self-Review Notes

- **Coverage:** parse/Merge/LoadLayered/GlobalConfigPath with global+local, only-global, neither (T1); CLI wired to LoadLayered at every call site; `GenerateAccounts` omitting stacks + `init --global` routing to the global path (T2).
- **Backward compatibility:** `config.Load` refactored to `parse`+`applyDefaults`+`Validate` with identical behavior; existing config tests untouched and must still pass. Only-local projects behave exactly as before.
- **Merge semantics:** local wins (accounts/groups by key, defaults per-field, stacks wholesale when local provides them); inputs not mutated (verified by test).
- **Lint hygiene:** no unused symbols; run staticcheck before each commit (CI lint is strict). If `GenerateAccounts` shares helpers with `Generate`, ensure no dead code remains.
- **Type consistency:** `parse`, `applyDefaults`, `Merge`, `LoadLayered`, `GlobalConfigPath`, `GenerateAccounts` used identically across tasks and the CLI.
