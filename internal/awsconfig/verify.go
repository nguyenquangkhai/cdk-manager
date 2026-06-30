package awsconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
)

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

// STSIdentity is the production IdentityFunc: it runs
// `aws sts get-caller-identity --profile <p> --output json` and parses .Account.
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
