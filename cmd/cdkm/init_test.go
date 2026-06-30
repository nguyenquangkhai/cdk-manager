package main

import (
	"bytes"
	"strings"
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

func TestCollectChoicesInteractive(t *testing.T) {
	profiles := []awsconfig.Profile{
		{Name: "a", Region: "r1", AccountID: "111111111111"},
		{Name: "b", Region: "r2", AccountID: "222222222222"},
	}
	input := "y\ndev,eu\ngroup1\ny\nprod\ngroup1\n"
	r := strings.NewReader(input)
	w := &bytes.Buffer{}

	choices := collectChoices(r, w, profiles, false)

	// Both should be included
	if !choices["a"].Include || !choices["b"].Include {
		t.Errorf("both profiles should be included: a.Include=%v, b.Include=%v", choices["a"].Include, choices["b"].Include)
	}

	// Check tags
	if len(choices["a"].Tags) != 2 || choices["a"].Tags[0] != "dev" || choices["a"].Tags[1] != "eu" {
		t.Errorf("a.Tags = %v, want [dev eu]", choices["a"].Tags)
	}
	if len(choices["b"].Tags) != 1 || choices["b"].Tags[0] != "prod" {
		t.Errorf("b.Tags = %v, want [prod]", choices["b"].Tags)
	}

	// Check groups
	if len(choices["a"].Groups) != 1 || choices["a"].Groups[0] != "group1" {
		t.Errorf("a.Groups = %v, want [group1]", choices["a"].Groups)
	}
	if len(choices["b"].Groups) != 1 || choices["b"].Groups[0] != "group1" {
		t.Errorf("b.Groups = %v, want [group1]", choices["b"].Groups)
	}

	// Verify the suggestion feature worked (output should contain "[existing: dev, eu]")
	output := w.String()
	if !strings.Contains(output, "[existing: dev, eu]") {
		t.Errorf("output missing suggestion hint for b's tags. Output:\n%s", output)
	}
}
