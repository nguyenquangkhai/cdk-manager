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
		force          bool
		toStdout       bool
		verify         bool
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
