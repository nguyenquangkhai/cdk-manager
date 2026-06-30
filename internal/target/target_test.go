package target

import (
	"testing"

	"github.com/nguyenquangkhai/cdk-manager/internal/config"
)

func fixture() *config.Config {
	return &config.Config{
		Accounts: map[string]config.Account{
			"dev-eu":  {Profile: "dev-eu", Region: "eu-west-1", Tags: []string{"dev", "eu"}, Context: map[string]string{"env": "dev"}},
			"prod-us": {Profile: "prod-us", Region: "us-east-1", Tags: []string{"prod", "us"}},
		},
		Groups: map[string]config.Group{
			"prod": {Tags: []string{"prod"}},
			"core": {Accounts: []string{"dev-eu", "prod-us"}},
		},
		Stacks: config.Stacks{
			Shared:     []string{"NetworkStack", "AppStack"},
			PerAccount: map[string][]string{"prod-us": {"ProdOnlyStack"}},
		},
	}
}

func TestResolveAll(t *testing.T) {
	got, err := Resolve(fixture(), Selector{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "dev-eu" || got[1].Name != "prod-us" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveByTag(t *testing.T) {
	got, _ := Resolve(fixture(), Selector{Tag: "prod"})
	if len(got) != 1 || got[0].Name != "prod-us" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveByGroupAccounts(t *testing.T) {
	got, _ := Resolve(fixture(), Selector{Group: "core"})
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveUnknownGroup(t *testing.T) {
	if _, err := Resolve(fixture(), Selector{Group: "ghost"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveByAccount(t *testing.T) {
	got, err := Resolve(fixture(), Selector{Account: "dev-eu"})
	if err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
	if len(got) != 1 || got[0].Name != "dev-eu" {
		t.Fatalf("got %+v, want 1 target named dev-eu", got)
	}
}

func TestResolveUnknownAccount(t *testing.T) {
	_, err := Resolve(fixture(), Selector{Account: "ghost"})
	if err == nil {
		t.Fatal("expected error for unknown account, got nil")
	}
}

func TestResolveTagZeroMatches(t *testing.T) {
	_, err := Resolve(fixture(), Selector{Tag: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for zero-match tag selector, got nil")
	}
}

func TestResolveGroupByTags(t *testing.T) {
	got, err := Resolve(fixture(), Selector{Group: "prod"})
	if err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
	if len(got) != 1 || got[0].Name != "prod-us" {
		t.Fatalf("got %+v, want 1 target named prod-us", got)
	}
}

func TestStacksMerge(t *testing.T) {
	c := fixture()
	prod := Target{Name: "prod-us"}
	got := Stacks(c, prod)
	want := []string{"NetworkStack", "AppStack", "ProdOnlyStack"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
