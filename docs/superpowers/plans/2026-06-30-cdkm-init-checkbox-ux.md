# cdkm — `init` checkbox UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Replace the per-account y/N prompt grind in `cdkm init` with a filterable checkbox multiselect (bubbletea) for picking accounts plus bulk tag assignment, and add an `--edit` flow that opens a prefilled `cdkm.yaml` in `$EDITOR`. Keep `--non-interactive`/`--stdout`/`--force`/`--verify`.

**Architecture:** A reusable, unit-testable bubbletea multiselect model in `internal/tui` (pure `Update(KeyMsg)` logic, no terminal needed for tests). The `init` command orchestrates: account multiselect → bulk-tag loop (reusing the same model) → `awsconfig.Generate`. A separate `--edit` path writes a prefilled doc, shells `$EDITOR`, and validates on return. Non-TTY input falls back to `--non-interactive`.

**Tech Stack:** Go 1.24, cobra, bubbletea (re-added; now actually used), gopkg.in/yaml.v3, existing `internal/awsconfig`.

## Global Constraints

- Module path: `github.com/nguyenquangkhai/cdk-manager` (verbatim in imports).
- Go floor stays **1.24**. After any `go get`, confirm `go.mod` still says `go 1.24`; pin any dep that raises it.
- The multiselect model must be testable WITHOUT a real terminal: drive it by calling `Update(tea.KeyMsg{...})` and asserting model state. No teatest dependency.
- Interactive TUI only runs on a TTY (`isTerminal()` already exists in cmd/cdkm). Non-TTY → behave as `--non-interactive`.
- Generated config must parse under `config.Load` (reuse `awsconfig.Generate`).
- Commit after each task; Conventional Commits. Body ends with:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- Run `gofmt -l`, `go vet`, `go test ./...` clean before every commit (lint is now enforced in CI; do not reintroduce dead/unused code — everything added must be referenced).

---

### Task 1: `internal/tui` filterable multiselect model

**Files:**
- Create: `internal/tui/multiselect.go`
- Test: `internal/tui/multiselect_test.go`

**Interfaces:**
- Produces:
  ```go
  package tui

  import tea "github.com/charmbracelet/bubbletea"

  // Item is one selectable row.
  type Item struct {
      Key  string // stable identifier returned on confirm (e.g. profile name)
      Desc string // right-hand description (e.g. "eu-west-1")
  }

  // Model is a filterable checkbox multiselect. Construct with NewModel,
  // run via tea.NewProgram, then read Selected() after it exits.
  // Keys: up/down or k/j move; space toggles; "a" toggles all (visible);
  // "/" enters filter mode (type to filter, esc/enter leaves filter mode);
  // enter confirms; "q"/ctrl+c aborts (sets Aborted()).
  type Model struct { /* unexported fields */ }

  func NewModel(title string, items []Item, preselectAll bool) Model

  func (m Model) Init() tea.Cmd
  func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)
  func (m Model) View() string

  // Selected returns the Keys of checked items, in original item order.
  func (m Model) Selected() []string
  // Aborted reports whether the user cancelled (q/ctrl+c) rather than confirming.
  func (m Model) Aborted() bool
  // for tests: visibleKeys returns the Keys currently passing the filter.
  func (m Model) visibleKeys() []string
  ```
- Behavior: filtering is a case-insensitive substring match on `Key`. Toggle-all affects only currently-visible (filtered) items. Cursor stays within the visible set.

- [ ] **Step 1: Write the failing test**

`internal/tui/multiselect_test.go`:

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func keys(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func items() []Item {
	return []Item{
		{Key: "prod-eu", Desc: "eu-west-1"},
		{Key: "prod-us", Desc: "us-east-1"},
		{Key: "dev-eu", Desc: "eu-west-1"},
	}
}

func TestToggleAndSelect(t *testing.T) {
	m := NewModel("Select", items(), false)
	// move to first item already at 0; toggle it.
	mm, _ := m.Update(keys(" "))
	m = mm.(Model)
	// down to index 2 (dev-eu) and toggle.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = mm.(Model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = mm.(Model)
	mm, _ = m.Update(keys(" "))
	m = mm.(Model)

	got := m.Selected()
	if len(got) != 2 || got[0] != "prod-eu" || got[1] != "dev-eu" {
		t.Fatalf("Selected()=%v want [prod-eu dev-eu] in item order", got)
	}
}

func TestPreselectAllAndToggleAll(t *testing.T) {
	m := NewModel("Select", items(), true)
	if len(m.Selected()) != 3 {
		t.Fatalf("preselectAll should check all, got %v", m.Selected())
	}
	// "a" toggles all visible off (all currently checked -> uncheck).
	mm, _ := m.Update(keys("a"))
	m = mm.(Model)
	if len(m.Selected()) != 0 {
		t.Fatalf("toggle-all should clear, got %v", m.Selected())
	}
}

func TestFilterLimitsToggleAllAndVisible(t *testing.T) {
	m := NewModel("Select", items(), false)
	// enter filter mode and type "prod"
	mm, _ := m.Update(keys("/"))
	m = mm.(Model)
	for _, r := range "prod" {
		mm, _ = m.Update(keys(string(r)))
		m = mm.(Model)
	}
	if vk := m.visibleKeys(); len(vk) != 2 || vk[0] != "prod-eu" || vk[1] != "prod-us" {
		t.Fatalf("visibleKeys=%v want [prod-eu prod-us]", vk)
	}
	// leave filter mode, toggle-all (affects only visible prod-*)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	mm, _ = m.Update(keys("a"))
	m = mm.(Model)
	got := m.Selected()
	if len(got) != 2 || got[0] != "prod-eu" || got[1] != "prod-us" {
		t.Fatalf("toggle-all under filter should select only prod-*, got %v", got)
	}
}

func TestAbort(t *testing.T) {
	m := NewModel("Select", items(), false)
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = mm.(Model)
	if !m.Aborted() {
		t.Fatal("ctrl+c should set Aborted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Add dep + implement**

Run first: `go get github.com/charmbracelet/bubbletea@v1.3.10` (confirm `go.mod` stays `go 1.24` after).

`internal/tui/multiselect.go`:

```go
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Item struct {
	Key  string
	Desc string
}

type Model struct {
	title    string
	items    []Item
	checked  map[int]bool // index into items
	cursor   int          // index into the visible slice
	filter   string
	filtering bool
	aborted  bool
	done     bool
}

func NewModel(title string, items []Item, preselectAll bool) Model {
	m := Model{title: title, items: items, checked: map[int]bool{}}
	if preselectAll {
		for i := range items {
			m.checked[i] = true
		}
	}
	return m
}

func (m Model) Init() tea.Cmd { return nil }

// visibleIdx returns item indices passing the current filter, in order.
func (m Model) visibleIdx() []int {
	var out []int
	f := strings.ToLower(m.filter)
	for i, it := range m.items {
		if f == "" || strings.Contains(strings.ToLower(it.Key), f) {
			out = append(out, i)
		}
	}
	return out
}

func (m Model) visibleKeys() []string {
	idx := m.visibleIdx()
	out := make([]string, len(idx))
	for i, j := range idx {
		out[i] = m.items[j].Key
	}
	return out
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	vis := m.visibleIdx()

	if m.filtering {
		switch km.Type {
		case tea.KeyEnter, tea.KeyEsc:
			m.filtering = false
		case tea.KeyBackspace:
			if n := len(m.filter); n > 0 {
				m.filter = m.filter[:n-1]
			}
		case tea.KeyRunes:
			m.filter += string(km.Runes)
		}
		if m.cursor >= len(m.visibleIdx()) {
			m.cursor = 0
		}
		return m, nil
	}

	switch {
	case km.Type == tea.KeyCtrlC || (km.Type == tea.KeyRunes && string(km.Runes) == "q"):
		m.aborted = true
		return m, tea.Quit
	case km.Type == tea.KeyEnter:
		m.done = true
		return m, tea.Quit
	case km.Type == tea.KeyUp || (km.Type == tea.KeyRunes && string(km.Runes) == "k"):
		if m.cursor > 0 {
			m.cursor--
		}
	case km.Type == tea.KeyDown || (km.Type == tea.KeyRunes && string(km.Runes) == "j"):
		if m.cursor < len(vis)-1 {
			m.cursor++
		}
	case km.Type == tea.KeyRunes && string(km.Runes) == " ":
		if m.cursor < len(vis) {
			i := vis[m.cursor]
			m.checked[i] = !m.checked[i]
		}
	case km.Type == tea.KeyRunes && string(km.Runes) == "a":
		// toggle-all over visible: if all visible checked, clear; else set.
		allChecked := len(vis) > 0
		for _, i := range vis {
			if !m.checked[i] {
				allChecked = false
				break
			}
		}
		for _, i := range vis {
			m.checked[i] = !allChecked
		}
	case km.Type == tea.KeyRunes && string(km.Runes) == "/":
		m.filtering = true
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(m.title) + "\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(
		"space toggle · /=filter · a=all · enter=confirm · q=cancel") + "\n")
	if m.filtering || m.filter != "" {
		b.WriteString("filter: " + m.filter + "\n")
	}
	b.WriteString("\n")
	for ci, i := range m.visibleIdx() {
		cursor := "  "
		if ci == m.cursor {
			cursor = "> "
		}
		box := "[ ]"
		if m.checked[i] {
			box = "[x]"
		}
		b.WriteString(cursor + box + " " + m.items[i].Key + "  " +
			lipgloss.NewStyle().Faint(true).Render(m.items[i].Desc) + "\n")
	}
	n := 0
	for _, v := range m.checked {
		if v {
			n++
		}
	}
	b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(
		strings.TrimSpace(itoa(n)+" selected")) + "\n")
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

func (m Model) Selected() []string {
	var out []string
	for i := range m.items {
		if m.checked[i] {
			out = append(out, m.items[i].Key)
		}
	}
	return out
}

func (m Model) Aborted() bool { return m.aborted }
```

- [ ] **Step 4: Run tests + floor check**

Run: `go test ./internal/tui/... && go vet ./internal/tui/... && grep '^go ' go.mod`
Expected: PASS; `go 1.24`.

- [ ] **Step 5: Commit**

```bash
git add internal/tui go.mod go.sum
git commit -m "feat: reusable filterable checkbox multiselect (bubbletea)"
```

---

### Task 2: wire checkbox flow into `cdkm init`

**Files:**
- Modify: `cmd/cdkm/init.go`
- Test: `cmd/cdkm/init_test.go`

**Interfaces:**
- Consumes: `tui.NewModel`, `tui.Item`, `tui.Model`; `awsconfig.Profile/Selection/Generate`.
- Produces a pure assembly helper, plus a `runProgram` seam so tests don't open a terminal:
  ```go
  // runSelect runs a multiselect over items and returns the chosen Keys (or
  // an error if the user aborted). Overridable in tests.
  var runSelect = func(title string, items []tui.Item, preselectAll bool) ([]string, bool, error) {
      m := tui.NewModel(title, items, preselectAll)
      out, err := tea.NewProgram(m).Run()
      if err != nil {
          return nil, false, err
      }
      fm := out.(tui.Model)
      return fm.Selected(), fm.Aborted(), nil
  }

  // interactiveTUI drives the full flow: pick accounts, then a bulk-tag loop
  // (prompt tag name via promptLine; multiselect which chosen accounts get it).
  // Returns Selections ready for awsconfig.Generate.
  func interactiveTUI(profiles []awsconfig.Profile, accountIDs map[string]string,
      promptLine func(prompt string) (string, bool)) ([]awsconfig.Selection, error)
  ```
- `interactiveTUI` uses the injectable `runSelect` and `promptLine` so it is unit-testable without a TTY.

- [ ] **Step 1: Write the failing test**

Append to `cmd/cdkm/init_test.go`:

```go
package main

import (
	"reflect"
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/awsconfig"
	"github.com/nguyenquangkhai/cdk-manager/internal/tui"
)

func TestInteractiveTUIBulkTags(t *testing.T) {
	profiles := []awsconfig.Profile{
		{Name: "prod-eu", Region: "eu-west-1"},
		{Name: "prod-us", Region: "us-east-1"},
		{Name: "dev-eu", Region: "eu-west-1", AccountID: "999"},
	}
	// Scripted selections: first call = account pick; subsequent = per-tag pick.
	calls := 0
	origRun := runSelect
	defer func() { runSelect = origRun }()
	runSelect = func(title string, items []tui.Item, preselectAll bool) ([]string, bool, error) {
		calls++
		switch calls {
		case 1: // account selection
			return []string{"prod-eu", "prod-us", "dev-eu"}, false, nil
		case 2: // accounts getting tag "prod"
			return []string{"prod-eu", "prod-us"}, false, nil
		default: // accounts getting tag "eu"
			return []string{"prod-eu", "dev-eu"}, false, nil
		}
	}
	// promptLine feeds tag names: "prod", "eu", then blank to finish.
	tagScript := []string{"prod", "eu", ""}
	ti := 0
	promptLine := func(string) (string, bool) {
		v := tagScript[ti]
		ti++
		return v, true
	}

	sels, err := interactiveTUI(profiles, map[string]string{}, promptLine)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]awsconfig.Selection{}
	for _, s := range sels {
		byName[s.Name] = s
	}
	if len(sels) != 3 {
		t.Fatalf("want 3 selections, got %d", len(sels))
	}
	if !reflect.DeepEqual(byName["prod-eu"].Tags, []string{"prod", "eu"}) {
		t.Errorf("prod-eu tags=%v want [prod eu]", byName["prod-eu"].Tags)
	}
	if !reflect.DeepEqual(byName["prod-us"].Tags, []string{"prod"}) {
		t.Errorf("prod-us tags=%v want [prod]", byName["prod-us"].Tags)
	}
	if !reflect.DeepEqual(byName["dev-eu"].Tags, []string{"eu"}) {
		t.Errorf("dev-eu tags=%v want [eu]", byName["dev-eu"].Tags)
	}
	if byName["dev-eu"].AccountID != "999" {
		t.Errorf("dev-eu should keep profile AccountID 999, got %q", byName["dev-eu"].AccountID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/cdkm/ -run TestInteractiveTUI`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

In `cmd/cdkm/init.go` add the `runSelect` var, `interactiveTUI`, and a default `promptLine` (reads a line from stdin via bufio, returns ok=false on EOF). Replace the old per-account `collectChoices` interactive path in `newInitCmd` with: if interactive (TTY, not `--non-interactive`, not `--edit`) call `interactiveTUI(profiles, accountIDs, promptLineStdin)`; else keep the `--non-interactive` behavior (all profiles, no tags). Keep all existing flags.

`interactiveTUI` logic:
```go
func interactiveTUI(profiles []awsconfig.Profile, accountIDs map[string]string,
	promptLine func(string) (string, bool)) ([]awsconfig.Selection, error) {

	items := make([]tui.Item, len(profiles))
	for i, p := range profiles {
		items[i] = tui.Item{Key: p.Name, Desc: p.Region}
	}
	chosen, aborted, err := runSelect("Select accounts to include", items, false)
	if err != nil {
		return nil, err
	}
	if aborted {
		return nil, fmt.Errorf("aborted")
	}
	chosenSet := map[string]bool{}
	for _, k := range chosen {
		chosenSet[k] = true
	}
	tags := map[string][]string{} // profile -> tags (in assignment order)

	// Bulk-tag loop: name a tag, pick which chosen accounts get it.
	chosenItems := make([]tui.Item, 0, len(chosen))
	for _, p := range profiles {
		if chosenSet[p.Name] {
			chosenItems = append(chosenItems, tui.Item{Key: p.Name, Desc: p.Region})
		}
	}
	for {
		name, ok := promptLine("Tag name to assign (blank to finish): ")
		if !ok || strings.TrimSpace(name) == "" {
			break
		}
		name = strings.TrimSpace(name)
		picked, ab, err := runSelect("Accounts to tag \""+name+"\"", chosenItems, false)
		if err != nil {
			return nil, err
		}
		if ab {
			continue
		}
		for _, pn := range picked {
			tags[pn] = append(tags[pn], name)
		}
	}

	var sels []awsconfig.Selection
	for _, p := range profiles {
		if !chosenSet[p.Name] {
			continue
		}
		acct := p.AccountID
		if id, ok := accountIDs[p.Name]; ok && id != "" {
			acct = id
		}
		sels = append(sels, awsconfig.Selection{
			Name: p.Name, Profile: p.Name, Region: p.Region,
			AccountID: acct, Tags: tags[p.Name],
		})
	}
	return sels, nil
}
```

- [ ] **Step 4: Run tests + full build**

Run: `go test ./cmd/cdkm/... && go build ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/cdkm
git commit -m "feat: checkbox multiselect + bulk tagging in cdkm init"
```

---

### Task 3: `--edit` ($EDITOR) flow + docs

**Files:**
- Modify: `cmd/cdkm/init.go`
- Test: `cmd/cdkm/init_test.go`
- Modify: `README.md`, `CHANGELOG.md`

**Interfaces:**
- Produces:
  ```go
  // allSelections builds Selections for every profile with empty tags
  // (the prefilled doc for --edit). accountIDs may override AccountID.
  func allSelections(profiles []awsconfig.Profile, accountIDs map[string]string) []awsconfig.Selection

  // editInEditor writes content to a temp file, runs `$EDITOR file`, and
  // returns the edited bytes. The command runner is injectable for tests.
  var editorRunner = func(path string) error { /* exec $EDITOR/$VISUAL/vi path */ }
  func editInEditor(content []byte) ([]byte, error)
  ```

- [ ] **Step 1: Write the failing test**

Append to `cmd/cdkm/init_test.go`:

```go
func TestAllSelectionsEmptyTags(t *testing.T) {
	profiles := []awsconfig.Profile{
		{Name: "a", Region: "r1", AccountID: "1"},
		{Name: "b", Region: "r2"},
	}
	sels := allSelections(profiles, map[string]string{"b": "2"})
	if len(sels) != 2 {
		t.Fatalf("got %d", len(sels))
	}
	for _, s := range sels {
		if len(s.Tags) != 0 {
			t.Errorf("%s should have empty tags, got %v", s.Name, s.Tags)
		}
	}
	byName := map[string]awsconfig.Selection{}
	for _, s := range sels {
		byName[s.Name] = s
	}
	if byName["a"].AccountID != "1" || byName["b"].AccountID != "2" {
		t.Errorf("account ids wrong: %+v", byName)
	}
}

func TestEditInEditorRoundTrip(t *testing.T) {
	orig := editorRunner
	defer func() { editorRunner = orig }()
	// Simulate an editor that appends a line.
	editorRunner = func(path string) error {
		b, _ := os.ReadFile(path)
		return os.WriteFile(path, append(b, []byte("\n# edited\n")...), 0o644)
	}
	out, err := editInEditor([]byte("accounts: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "# edited") {
		t.Errorf("editor changes not returned: %q", out)
	}
}
```

(Ensure `os` and `strings` are imported in the test file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/cdkm/ -run 'TestAllSelections|TestEditInEditor'`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `--edit`**

Add `allSelections`, `editorRunner` (resolves `$VISUAL`→`$EDITOR`→`vi`, runs `exec.Command(editor, path)` with stdio inherited), and `editInEditor` (temp file write → `editorRunner` → read back). Wire `--edit` into `newInitCmd`:

- `--edit` path: build `allSelections(profiles, accountIDs)` → `awsconfig.Generate` → `editInEditor(doc)` → validate the result by `yaml.Unmarshal` into the config schema (or call into `config.Load` via a temp file) → on parse error, report it and keep the raw edited bytes for the user to fix; on success, write to `cdkm.yaml` (respect `--force`/`--stdout`).
- Resolve precedence in `newInitCmd`: `--stdout` and `--force` apply to output as before. If `--edit`, skip the TUI/non-interactive selection entirely (editor IS the selection). If not `--edit`: TTY & !`--non-interactive` → `interactiveTUI`; else non-interactive all-profiles.
- Non-TTY guard: if interactive TUI would run but stdout/stdin is not a TTY, print a hint to use `--non-interactive` or `--edit` and return a clean error.

- [ ] **Step 4: Run tests + full verify**

Run: `go test ./... && go build ./... && go vet ./... && gofmt -l . | grep -v testdata`
Expected: tests PASS; no gofmt output.

- [ ] **Step 5: Docs**

README: update the `cdkm init` section to describe the checkbox flow (space/filter/all/enter), bulk tagging, and `--edit`/`--non-interactive`. CHANGELOG `## [Unreleased]` → Added: checkbox multiselect + bulk tagging in `cdkm init`; `--edit` opens a prefilled config in `$EDITOR`.

- [ ] **Step 6: Manual smoke (document, do not require TTY in CI)**

`go run ./cmd/cdkm init --non-interactive --stdout` (fixtures) still works; note in the report that the TUI path is exercised by unit tests via injected `runSelect`/`promptLine`, and `--edit` via injected `editorRunner`.

- [ ] **Step 7: Commit + push**

```bash
git add cmd/cdkm README.md CHANGELOG.md
git commit -m "feat: cdkm init --edit ($EDITOR) flow and docs"
git push origin main
```

---

## Self-Review Notes

- **Coverage:** multiselect model toggle/filter/toggle-all/abort (T1); full interactive assembly with bulk tags via injected `runSelect`/`promptLine` (T2); `allSelections` + `editInEditor` via injected `editorRunner`, plus `--edit` wiring and docs (T3).
- **Testability without a TTY:** `runSelect`, `promptLine`, and `editorRunner` are package-level vars/params injected in tests; the bubbletea `Model` is tested by feeding `tea.KeyMsg` to `Update`. No terminal, no teatest dep.
- **No dead code:** bubbletea is re-added but now referenced by `internal/tui` and `init.go` (the reason it was removed before was that it was unused — it no longer is). Confirm `staticcheck ./...` is clean before each commit.
- **Floor guard:** confirm `go 1.24` after `go get bubbletea`.
- **Type consistency:** `tui.Item/Model/NewModel/Selected/Aborted`, `runSelect`, `interactiveTUI`, `allSelections`, `editInEditor`, `editorRunner`, and `awsconfig.Selection` are used identically across T1–T3.
- **Non-TTY fallback:** interactive path guarded by `isTerminal()`; piped/CI use `--non-interactive` or `--edit`.
```
