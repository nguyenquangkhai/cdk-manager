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
