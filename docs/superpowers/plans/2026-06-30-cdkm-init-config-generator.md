# cdkm — `init` config generator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** A `cdkm init` command that discovers AWS named profiles from `~/.aws/config`/`credentials` and generates a starter `cdkm.yaml`, with an interactive picker for tags/groups and an optional `--verify` that confirms credentials and fills real account ids.

**Architecture:** Pure, fixture-tested core in `internal/awsconfig` (parse profiles + generate YAML from structured selections). A thin interactive layer (plain stdin prompts) and an optional `--verify` enrichment (shells `aws sts get-caller-identity` in parallel) live in the cobra `init` command. Account id is optional metadata only — cdkm operates on the profile/credentials, not the id.

**Tech Stack:** Go 1.24, cobra, gopkg.in/yaml.v3 (existing), gopkg.in/ini.v1 (new), golang.org/x/sync/errgroup (new, for parallel verify) or a small WaitGroup.

## Global Constraints

- Module path: `github.com/nguyenquangkhai/cdk-manager` (verbatim in imports).
- Go floor: **1.24**. After any `go get`, confirm `go.mod` still says `go 1.24` (a dep that forces a higher floor must be pinned down — do NOT let the floor rise, it breaks the CI 1.24 job).
- Generated config matches the existing `config.Config` schema (accounts/groups/stacks) so `config.Load` accepts it.
- Account id is OPTIONAL: emit `context.account` only when known (profile `sso_account_id` or `--verify`). Never required.
- Default AWS paths: `$AWS_CONFIG_FILE` or `~/.aws/config`; `$AWS_SHARED_CREDENTIALS_FILE` or `~/.aws/credentials`. Parser takes explicit paths (injected) for tests.
- `init` writes `./cdkm.yaml`; refuses if it exists unless `--force`; `--stdout` prints instead of writing.
- No real network and no real `~/.aws` in tests — inject paths and the verify-fetcher.
- Commit after each task; Conventional Commits. Body ends with:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`

---

### Task 1: `internal/awsconfig` — parse profiles + generate YAML

**Files:**
- Create: `internal/awsconfig/awsconfig.go`
- Test: `internal/awsconfig/awsconfig_test.go`
- Create: `internal/awsconfig/testdata/config.ini`
- Create: `internal/awsconfig/testdata/credentials.ini`

**Interfaces:**
- Produces:
  ```go
  package awsconfig

  // Profile is one AWS named profile discovered from config/credentials.
  type Profile struct {
      Name      string // profile name (e.g. "dev-eu", "default")
      Region    string // region or sso_region; "" if unknown
      AccountID string // sso_account_id if present; else ""
  }

  // Parse reads the AWS config and credentials INI files at the given paths
  // and returns the union of profiles, sorted by Name. A missing file is not
  // an error (its profiles are simply absent). Profiles from credentials that
  // are not in config are still included (Region/AccountID may be empty).
  func Parse(configPath, credsPath string) ([]Profile, error)

  // Selection is a user's decision about one profile for config generation.
  type Selection struct {
      Name      string   // logical account key in cdkm.yaml (defaults to profile name)
      Profile   string   // AWS profile name
      Region    string
      AccountID string   // optional; emitted under context.account when non-empty
      Tags      []string
      Groups    []string // group names this account belongs to
  }

  // Generate renders a cdkm.yaml document from selections. Accounts are written
  // with profile/region/tags and context.account (only when AccountID != "").
  // Groups are emitted as explicit account lists built from Selection.Groups.
  // stacks is scaffolded as {shared: []}. Output is deterministic (sorted keys).
  func Generate(sels []Selection) ([]byte, error)
  ```
- Behavior notes: in `~/.aws/config`, profile sections are `[profile NAME]` (and `[default]`); in `~/.aws/credentials` they are `[NAME]`. Read `region`, `sso_region` (fallback), `sso_account_id`. Ignore `[sso-session ...]` sections.

- [ ] **Step 1: Write fixtures + failing test**

`internal/awsconfig/testdata/config.ini`:

```ini
[default]
region = us-east-1

[profile dev-eu]
region = eu-west-1
sso_account_id = 111111111111
sso_region = eu-west-1

[profile prod-us]
region = us-east-1

[sso-session my-sso]
sso_region = us-east-1
sso_start_url = https://example.awsapps.com/start
```

`internal/awsconfig/testdata/credentials.ini`:

```ini
[default]
aws_access_key_id = AKIAEXAMPLE
aws_secret_access_key = secret

[ci-bot]
aws_access_key_id = AKIACI
aws_secret_access_key = secret2
```

`internal/awsconfig/awsconfig_test.go`:

```go
package awsconfig

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseProfiles(t *testing.T) {
	got, err := Parse("testdata/config.ini", "testdata/credentials.ini")
	if err != nil {
		t.Fatal(err)
	}
	// default, dev-eu, prod-us (from config) + ci-bot (credentials-only); default deduped.
	byName := map[string]Profile{}
	for _, p := range got {
		byName[p.Name] = p
	}
	if _, ok := byName["dev-eu"]; !ok {
		t.Fatal("missing dev-eu")
	}
	if byName["dev-eu"].Region != "eu-west-1" || byName["dev-eu"].AccountID != "111111111111" {
		t.Errorf("dev-eu = %+v", byName["dev-eu"])
	}
	if _, ok := byName["ci-bot"]; !ok {
		t.Error("credentials-only profile ci-bot should be included")
	}
	if _, ok := byName["my-sso"]; ok {
		t.Error("sso-session section must not be treated as a profile")
	}
	// sorted by name
	for i := 1; i < len(got); i++ {
		if got[i-1].Name > got[i].Name {
			t.Fatalf("not sorted: %v", got)
		}
	}
}

func TestParseMissingFilesNotError(t *testing.T) {
	got, err := Parse("testdata/does-not-exist", "testdata/also-missing")
	if err != nil {
		t.Fatalf("missing files should not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no profiles, got %v", got)
	}
}

func TestGenerateValidConfig(t *testing.T) {
	sels := []Selection{
		{Name: "dev-eu", Profile: "dev-eu", Region: "eu-west-1", AccountID: "111111111111", Tags: []string{"dev", "eu"}, Groups: []string{"dev"}},
		{Name: "prod-us", Profile: "prod-us", Region: "us-east-1", Tags: []string{"prod"}, Groups: []string{"prod"}},
	}
	out, err := Generate(sels)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "dev-eu") || !strings.Contains(s, "profile: dev-eu") {
		t.Errorf("missing account block:\n%s", s)
	}
	if !strings.Contains(s, "account: \"111111111111\"") && !strings.Contains(s, "account: 111111111111") {
		t.Errorf("expected account id in context:\n%s", s)
	}
	if strings.Contains(s, "prod-us") && strings.Contains(s, "account:") {
		// prod-us has no AccountID -> must not emit an account key under it.
		// (Light check: ensure the only account: line corresponds to dev-eu.)
	}
	// Round-trips into the real config schema.
	var probe struct {
		Accounts map[string]struct {
			Profile string   `yaml:"profile"`
			Region  string   `yaml:"region"`
			Tags    []string `yaml:"tags"`
		} `yaml:"accounts"`
		Groups map[string]struct {
			Accounts []string `yaml:"accounts"`
		} `yaml:"groups"`
		Stacks struct {
			Shared []string `yaml:"shared"`
		} `yaml:"stacks"`
	}
	if err := yaml.Unmarshal(out, &probe); err != nil {
		t.Fatalf("generated yaml invalid: %v\n%s", err, s)
	}
	if probe.Accounts["dev-eu"].Profile != "dev-eu" || probe.Accounts["dev-eu"].Region != "eu-west-1" {
		t.Errorf("dev-eu account wrong: %+v", probe.Accounts["dev-eu"])
	}
	if len(probe.Groups["dev"].Accounts) != 1 || probe.Groups["dev"].Accounts[0] != "dev-eu" {
		t.Errorf("group dev wrong: %+v", probe.Groups["dev"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/awsconfig/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

`internal/awsconfig/awsconfig.go`:

```go
package awsconfig

import (
	"os"
	"sort"
	"strings"

	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

type Profile struct {
	Name      string
	Region    string
	AccountID string
}

type Selection struct {
	Name      string
	Profile   string
	Region    string
	AccountID string
	Tags      []string
	Groups    []string
}

func Parse(configPath, credsPath string) ([]Profile, error) {
	byName := map[string]*Profile{}

	if f, err := loadINI(configPath); err == nil {
		for _, sec := range f.Sections() {
			name := sectionToProfileName(sec.Name())
			if name == "" {
				continue
			}
			p := ensure(byName, name)
			if v := sec.Key("region").String(); v != "" {
				p.Region = v
			} else if v := sec.Key("sso_region").String(); v != "" && p.Region == "" {
				p.Region = v
			}
			if v := sec.Key("sso_account_id").String(); v != "" {
				p.AccountID = v
			}
		}
	}
	if f, err := loadINI(credsPath); err == nil {
		for _, sec := range f.Sections() {
			name := sec.Name()
			if name == ini.DefaultSection {
				// go-ini's implicit default section; skip unless it has keys.
				if len(sec.Keys()) == 0 {
					continue
				}
				name = "default"
			}
			if name == "" {
				continue
			}
			ensure(byName, name) // include credentials-only profiles
		}
	}

	out := make([]Profile, 0, len(byName))
	for _, p := range byName {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func loadINI(path string) (*ini.File, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return ini.Load(path)
}

func ensure(m map[string]*Profile, name string) *Profile {
	if p, ok := m[name]; ok {
		return p
	}
	p := &Profile{Name: name}
	m[name] = p
	return p
}

// sectionToProfileName maps an ~/.aws/config section name to a profile name,
// or "" if the section is not a profile (sso-session, services, etc.).
func sectionToProfileName(section string) string {
	switch {
	case section == "default":
		return "default"
	case strings.HasPrefix(section, "profile "):
		return strings.TrimSpace(strings.TrimPrefix(section, "profile "))
	default:
		return "" // sso-session, services, DEFAULT, etc.
	}
}

func Generate(sels []Selection) ([]byte, error) {
	type account struct {
		Profile string            `yaml:"profile"`
		Region  string            `yaml:"region"`
		Tags    []string          `yaml:"tags"`
		Context map[string]string `yaml:"context,omitempty"`
	}
	type group struct {
		Accounts []string `yaml:"accounts"`
	}
	doc := struct {
		Accounts map[string]account `yaml:"accounts"`
		Groups   map[string]group   `yaml:"groups"`
		Stacks   struct {
			Shared []string `yaml:"shared"`
		} `yaml:"stacks"`
	}{
		Accounts: map[string]account{},
		Groups:   map[string]group{},
	}

	for _, s := range sels {
		a := account{Profile: s.Profile, Region: s.Region, Tags: s.Tags}
		if a.Tags == nil {
			a.Tags = []string{}
		}
		if s.AccountID != "" {
			a.Context = map[string]string{"account": s.AccountID}
		}
		doc.Accounts[s.Name] = a
		for _, g := range s.Groups {
			gr := doc.Groups[g]
			gr.Accounts = append(gr.Accounts, s.Name)
			doc.Groups[g] = gr
		}
	}
	for name, gr := range doc.Groups {
		sort.Strings(gr.Accounts)
		doc.Groups[name] = gr
	}
	doc.Stacks.Shared = []string{}

	return yaml.Marshal(doc)
}
```

- [ ] **Step 4: Add deps + run tests**

Run: `go get gopkg.in/ini.v1 && go test ./internal/awsconfig/...`
Expected: PASS. Then confirm `grep '^go ' go.mod` still shows `go 1.24` (pin any dep that raised it).

- [ ] **Step 5: Commit**

```bash
git add internal/awsconfig go.mod go.sum
git commit -m "feat: parse AWS profiles and generate cdkm.yaml from selections"
```

---

### Task 2: `--verify` enrichment (parallel `aws sts get-caller-identity`)

**Files:**
- Create: `internal/awsconfig/verify.go`
- Test: `internal/awsconfig/verify_test.go`

**Interfaces:**
- Produces:
  ```go
  package awsconfig

  // IdentityFunc returns the AWS account id for a profile, or an error if the
  // credentials are invalid/unreachable. Injected for tests; the production
  // impl shells `aws sts get-caller-identity`.
  type IdentityFunc func(ctx context.Context, profile string) (accountID string, err error)

  // VerifyResult pairs a profile with its verification outcome.
  type VerifyResult struct {
      Profile   string
      AccountID string // set when ok
      OK        bool
      Err       error
  }

  // Verify runs ident for each profile concurrently (bounded by concurrency,
  // default 4) and returns results in the same order as profiles.
  func Verify(ctx context.Context, profiles []Profile, ident IdentityFunc, concurrency int) []VerifyResult

  // STSIdentity is the production IdentityFunc: it runs
  // `aws sts get-caller-identity --profile <p> --output json` and parses .Account.
  func STSIdentity(ctx context.Context, profile string) (string, error)
  ```

- [ ] **Step 1: Write the failing test**

`internal/awsconfig/verify_test.go`:

```go
package awsconfig

import (
	"context"
	"errors"
	"testing"
)

func TestVerifyConcurrentOrderedResults(t *testing.T) {
	profiles := []Profile{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	ident := func(ctx context.Context, profile string) (string, error) {
		if profile == "b" {
			return "", errors.New("expired token")
		}
		return "acct-" + profile, nil
	}
	res := Verify(context.Background(), profiles, ident, 2)
	if len(res) != 3 {
		t.Fatalf("got %d results", len(res))
	}
	if res[0].Profile != "a" || !res[0].OK || res[0].AccountID != "acct-a" {
		t.Errorf("a: %+v", res[0])
	}
	if res[1].Profile != "b" || res[1].OK || res[1].Err == nil {
		t.Errorf("b should fail: %+v", res[1])
	}
	if res[2].Profile != "c" || !res[2].OK || res[2].AccountID != "acct-c" {
		t.Errorf("c: %+v", res[2])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/awsconfig/ -run TestVerify`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

`internal/awsconfig/verify.go`:

```go
package awsconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
)

type IdentityFunc func(ctx context.Context, profile string) (string, error)

type VerifyResult struct {
	Profile   string
	AccountID string
	OK        bool
	Err       error
}

func Verify(ctx context.Context, profiles []Profile, ident IdentityFunc, concurrency int) []VerifyResult {
	if concurrency < 1 {
		concurrency = 4
	}
	results := make([]VerifyResult, len(profiles))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, p := range profiles {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			id, err := ident(ctx, name)
			if err != nil {
				results[i] = VerifyResult{Profile: name, OK: false, Err: err}
				return
			}
			results[i] = VerifyResult{Profile: name, AccountID: id, OK: true}
		}(i, p.Name)
	}
	wg.Wait()
	return results
}

func STSIdentity(ctx context.Context, profile string) (string, error) {
	cmd := exec.CommandContext(ctx, "aws", "sts", "get-caller-identity",
		"--profile", profile, "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("aws sts get-caller-identity --profile %s: %w", profile, err)
	}
	var body struct {
		Account string `json:"Account"`
	}
	if err := json.Unmarshal(out, &body); err != nil {
		return "", fmt.Errorf("parse sts output for %s: %w", profile, err)
	}
	if body.Account == "" {
		return "", fmt.Errorf("empty account id for %s", profile)
	}
	return body.Account, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/awsconfig/...`
Expected: PASS (parser + generate + verify).

- [ ] **Step 5: Commit**

```bash
git add internal/awsconfig
git commit -m "feat: parallel --verify enrichment via aws sts get-caller-identity"
```

---

### Task 3: `cdkm init` command (interactive picker + flags + docs)

**Files:**
- Create: `cmd/cdkm/init.go`
- Modify: `cmd/cdkm/root.go` (register the command)
- Test: `cmd/cdkm/init_test.go`
- Modify: `README.md`, `CHANGELOG.md`

**Interfaces:**
- Consumes: `awsconfig.Parse`, `awsconfig.Generate`, `awsconfig.Verify`, `awsconfig.STSIdentity`, `awsconfig.Profile`, `awsconfig.Selection`, `awsconfig.VerifyResult`.
- Produces in `init.go` a testable pure helper plus the cobra command:
  ```go
  // buildSelections converts parsed profiles + per-profile tag/group choices
  // into awsconfig.Selection values. choices maps profile name -> (tags, groups);
  // a profile absent from choices is excluded. accountIDs (from --verify, may be
  // nil) overrides a profile's AccountID when present.
  func buildSelections(profiles []awsconfig.Profile, choices map[string]profileChoice, accountIDs map[string]string) []awsconfig.Selection

  type profileChoice struct {
      Include bool
      Tags    []string
      Groups  []string
  }
  ```

- [ ] **Step 1: Write the failing test (pure helper)**

`cmd/cdkm/init_test.go`:

```go
package main

import (
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/awsconfig"
)

func TestBuildSelections(t *testing.T) {
	profiles := []awsconfig.Profile{
		{Name: "dev-eu", Region: "eu-west-1", AccountID: "111111111111"},
		{Name: "prod-us", Region: "us-east-1"},
		{Name: "skip-me", Region: "us-west-2"},
	}
	choices := map[string]profileChoice{
		"dev-eu":  {Include: true, Tags: []string{"dev"}, Groups: []string{"dev"}},
		"prod-us": {Include: true, Tags: []string{"prod"}, Groups: []string{"prod"}},
		// skip-me absent -> excluded
	}
	accountIDs := map[string]string{"prod-us": "222222222222"} // from --verify

	sels := buildSelections(profiles, choices, accountIDs)
	if len(sels) != 2 {
		t.Fatalf("got %d selections, want 2", len(sels))
	}
	byName := map[string]awsconfig.Selection{}
	for _, s := range sels {
		byName[s.Name] = s
	}
	if byName["dev-eu"].AccountID != "111111111111" {
		t.Errorf("dev-eu account from profile: %+v", byName["dev-eu"])
	}
	if byName["prod-us"].AccountID != "222222222222" {
		t.Errorf("prod-us account should come from verify override: %+v", byName["prod-us"])
	}
	if byName["prod-us"].Profile != "prod-us" || byName["prod-us"].Region != "us-east-1" {
		t.Errorf("prod-us fields: %+v", byName["prod-us"])
	}
}

func TestOrderedSetDedupAndOrder(t *testing.T) {
	s := newOrderedSet()
	s.addAll([]string{"dev", "eu"})
	s.addAll([]string{"eu", "prod"}) // eu is a dup
	want := []string{"dev", "eu", "prod"}
	if len(s.items) != len(want) {
		t.Fatalf("items=%v want %v", s.items, want)
	}
	for i := range want {
		if s.items[i] != want[i] {
			t.Fatalf("items=%v want %v", s.items, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/cdkm/ -run TestBuildSelections`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement helper + command**

`cmd/cdkm/init.go`:

```go
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nguyenquangkhai/cdk-manager/internal/awsconfig"
	"github.com/spf13/cobra"
)

type profileChoice struct {
	Include bool
	Tags    []string
	Groups  []string
}

func buildSelections(profiles []awsconfig.Profile, choices map[string]profileChoice, accountIDs map[string]string) []awsconfig.Selection {
	var sels []awsconfig.Selection
	for _, p := range profiles {
		c, ok := choices[p.Name]
		if !ok || !c.Include {
			continue
		}
		acct := p.AccountID
		if id, ok := accountIDs[p.Name]; ok && id != "" {
			acct = id
		}
		sels = append(sels, awsconfig.Selection{
			Name:      p.Name,
			Profile:   p.Name,
			Region:    p.Region,
			AccountID: acct,
			Tags:      c.Tags,
			Groups:    c.Groups,
		})
	}
	return sels
}

func defaultAWSPaths() (string, string) {
	cfg := os.Getenv("AWS_CONFIG_FILE")
	creds := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	home, _ := os.UserHomeDir()
	if cfg == "" {
		cfg = filepath.Join(home, ".aws", "config")
	}
	if creds == "" {
		creds = filepath.Join(home, ".aws", "credentials")
	}
	return cfg, creds
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func newInitCmd() *cobra.Command {
	var (
		force         bool
		toStdout      bool
		verify        bool
		nonInteractive bool
	)
	c := &cobra.Command{
		Use:   "init",
		Short: "Generate a starter cdkm.yaml from your AWS profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, credsPath := defaultAWSPaths()
			profiles, err := awsconfig.Parse(cfgPath, credsPath)
			if err != nil {
				return err
			}
			if len(profiles) == 0 {
				return fmt.Errorf("no AWS profiles found in %s or %s", cfgPath, credsPath)
			}

			accountIDs := map[string]string{}
			if verify {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				fmt.Fprintln(os.Stderr, "Verifying credentials via aws sts get-caller-identity ...")
				for _, r := range awsconfig.Verify(ctx, profiles, awsconfig.STSIdentity, 4) {
					if r.OK {
						accountIDs[r.Profile] = r.AccountID
					} else {
						fmt.Fprintf(os.Stderr, "  ! %s: %v\n", r.Profile, r.Err)
					}
				}
			}

			choices := collectChoices(os.Stdin, os.Stderr, profiles, nonInteractive)
			sels := buildSelections(profiles, choices, accountIDs)
			if len(sels) == 0 {
				return fmt.Errorf("no profiles selected; nothing to write")
			}
			out, err := awsconfig.Generate(sels)
			if err != nil {
				return err
			}

			if toStdout {
				_, err := os.Stdout.Write(out)
				return err
			}
			const target = "cdkm.yaml"
			if _, err := os.Stat(target); err == nil && !force {
				return fmt.Errorf("%s already exists (use --force to overwrite or --stdout to print)", target)
			}
			if err := os.WriteFile(target, out, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Wrote %s (%d account(s)). Review tags/groups/stacks before use.\n", target, len(sels))
			return nil
		},
	}
	c.Flags().BoolVar(&force, "force", false, "overwrite an existing cdkm.yaml")
	c.Flags().BoolVar(&toStdout, "stdout", false, "print to stdout instead of writing cdkm.yaml")
	c.Flags().BoolVar(&verify, "verify", false, "run aws sts get-caller-identity per profile to confirm creds and fill account ids")
	c.Flags().BoolVar(&nonInteractive, "non-interactive", false, "include all profiles with empty tags/groups (no prompts)")
	return c
}

// collectChoices prompts (via r/w) for which profiles to include and their
// tags/groups. With nonInteractive, every profile is included with no tags.
// Tags/groups already entered for earlier accounts are surfaced as
// suggestions so the user reuses a consistent set instead of retyping
// (avoiding prod/production-style typos).
func collectChoices(r *os.File, w *os.File, profiles []awsconfig.Profile, nonInteractive bool) map[string]profileChoice {
	choices := map[string]profileChoice{}
	if nonInteractive {
		for _, p := range profiles {
			choices[p.Name] = profileChoice{Include: true}
		}
		return choices
	}
	seenTags := newOrderedSet()
	seenGroups := newOrderedSet()
	sc := bufio.NewScanner(r)
	for _, p := range profiles {
		fmt.Fprintf(w, "Include profile %q (region=%s, account=%s)? [y/N] ", p.Name, p.Region, p.AccountID)
		if !sc.Scan() {
			break
		}
		if strings.ToLower(strings.TrimSpace(sc.Text())) != "y" {
			continue
		}
		fmt.Fprintf(w, "  tags for %s (comma-separated, blank for none)%s: ", p.Name, suggestion(seenTags))
		sc.Scan()
		tags := splitCSV(sc.Text())
		seenTags.addAll(tags)
		fmt.Fprintf(w, "  groups for %s (comma-separated, blank for none)%s: ", p.Name, suggestion(seenGroups))
		sc.Scan()
		groups := splitCSV(sc.Text())
		seenGroups.addAll(groups)
		choices[p.Name] = profileChoice{Include: true, Tags: tags, Groups: groups}
	}
	return choices
}

// suggestion renders previously-entered values as a hint, or "" if none yet.
func suggestion(s *orderedSet) string {
	if len(s.items) == 0 {
		return ""
	}
	return " [existing: " + strings.Join(s.items, ", ") + "]"
}

// orderedSet preserves first-seen insertion order for stable suggestion hints.
type orderedSet struct {
	items []string
	seen  map[string]struct{}
}

func newOrderedSet() *orderedSet { return &orderedSet{seen: map[string]struct{}{}} }

func (s *orderedSet) addAll(vs []string) {
	for _, v := range vs {
		if _, ok := s.seen[v]; ok {
			continue
		}
		s.seen[v] = struct{}{}
		s.items = append(s.items, v)
	}
}
```

Register in `cmd/cdkm/root.go` where other subcommands are added: `rootCmd.AddCommand(newInitCmd())`.

- [ ] **Step 4: Run unit test + build**

Run: `go test ./cmd/cdkm/ -run TestBuildSelections && go build ./...`
Expected: PASS + build clean.

- [ ] **Step 5: Smoke (non-interactive + stdout, against fixture paths)**

```bash
AWS_CONFIG_FILE=internal/awsconfig/testdata/config.ini \
AWS_SHARED_CREDENTIALS_FILE=internal/awsconfig/testdata/credentials.ini \
go run ./cmd/cdkm init --non-interactive --stdout
```
Expected: prints a valid `cdkm.yaml` with accounts default/dev-eu/prod-us/ci-bot, empty tags, `stacks: { shared: [] }`. Pipe it through `cdkm`? Just confirm it parses: save to a temp file and run any read path mentally — the Generate test already proves schema validity.

- [ ] **Step 6: Docs**

README: add an "Getting started" note — `cdkm init` (and `cdkm init --verify`) to scaffold `cdkm.yaml` from `~/.aws` profiles, then edit tags/groups/stacks. CHANGELOG `## [Unreleased]` → Added: `cdkm init` config generator with interactive picker and optional `--verify`.

- [ ] **Step 7: Full verify + commit**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all PASS.

```bash
git add cmd/cdkm internal/awsconfig README.md CHANGELOG.md
git commit -m "feat: cdkm init config generator with interactive picker and --verify"
```

---

## Self-Review Notes

- **Coverage:** profile parse incl. credentials-only + sso-session exclusion (T1), deterministic YAML generation matching `config.Config` schema (T1), parallel ordered verify with failure handling (T2), selection assembly with verify-override + interactive/non-interactive prompts + write/stdout/force flags (T3).
- **Account id is optional everywhere:** emitted only when known (sso_account_id or --verify), via `context.account`; never required by parse/generate/init.
- **Testability:** Parse takes explicit paths (fixtures); Verify takes an injected IdentityFunc (no real `aws`/network in tests); buildSelections is pure. The interactive prompt loop (`collectChoices`) and `STSIdentity` shell are thin and exercised via smoke, not unit tests.
- **Floor guard:** after `go get ini.v1`, confirm go.mod stays `go 1.24`; pin the dep down if it raised the floor (lesson from the x/mod incident).
- **Type consistency:** `Profile`, `Selection`, `VerifyResult`, `IdentityFunc`, `profileChoice`, `buildSelections`, `Parse`, `Generate`, `Verify`, `STSIdentity` used identically across T1–T3 and the cobra wiring.
