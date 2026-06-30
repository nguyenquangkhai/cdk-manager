package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/awsconfig"
	"github.com/nguyenquangkhai/cdk-manager/internal/tui"
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

func TestBuildSelectionsExcludesIncludeFalse(t *testing.T) {
	profiles := []awsconfig.Profile{
		{Name: "include-me", Region: "us-east-1", AccountID: "111111111111"},
		{Name: "exclude-me", Region: "us-west-2", AccountID: "222222222222"},
	}
	choices := map[string]profileChoice{
		"include-me": {Include: true, Tags: []string{"keep"}},
		"exclude-me": {Include: false, Tags: []string{"skip"}},
	}
	accountIDs := map[string]string{}

	sels := buildSelections(profiles, choices, accountIDs)
	if len(sels) != 1 {
		t.Fatalf("got %d selections, want 1", len(sels))
	}
	if sels[0].Name != "include-me" {
		t.Errorf("got %q, want include-me", sels[0].Name)
	}
}

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
