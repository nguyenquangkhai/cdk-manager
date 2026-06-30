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
