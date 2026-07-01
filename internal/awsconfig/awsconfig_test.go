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
