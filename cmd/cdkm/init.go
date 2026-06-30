package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/nguyenquangkhai/cdk-manager/internal/awsconfig"
	"github.com/nguyenquangkhai/cdk-manager/internal/config"
	"github.com/nguyenquangkhai/cdk-manager/internal/tui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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

// allSelections builds a Selection for every profile with empty tags —
// the prefilled document used for --edit. accountIDs may override AccountID.
func allSelections(profiles []awsconfig.Profile, accountIDs map[string]string) []awsconfig.Selection {
	sels := make([]awsconfig.Selection, 0, len(profiles))
	for _, p := range profiles {
		acct := p.AccountID
		if id, ok := accountIDs[p.Name]; ok && id != "" {
			acct = id
		}
		sels = append(sels, awsconfig.Selection{
			Name:      p.Name,
			Profile:   p.Name,
			Region:    p.Region,
			AccountID: acct,
			Tags:      nil,
			Groups:    nil,
		})
	}
	return sels
}

// editorRunner opens path in the user's preferred editor.
// Overridable in tests.
var editorRunner = func(path string) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// validateConfigBytes checks that edited init output parses AND passes the
// config schema validation (non-empty accounts, known perAccount/group refs).
func validateConfigBytes(b []byte) error {
	var cfg config.Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	return nil
}

// editInEditor writes content to a temp file, invokes editorRunner, and
// returns the edited bytes.
func editInEditor(content []byte) ([]byte, error) {
	f, err := os.CreateTemp("", "cdkm-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(content); err != nil {
		f.Close()
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}
	if err := editorRunner(f.Name()); err != nil {
		return nil, fmt.Errorf("editor: %w", err)
	}
	edited, err := os.ReadFile(f.Name())
	if err != nil {
		return nil, fmt.Errorf("read edited file: %w", err)
	}
	return edited, nil
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

// promptLineStdin reads a line from stdin. Returns ("", false) on EOF.
func promptLineStdin(prompt string) (string, bool) {
	fmt.Print(prompt)
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return "", false
	}
	return sc.Text(), true
}

// interactiveTUI drives the full flow: pick accounts, then a bulk-tag loop
// (prompt tag name via promptLine; multiselect which chosen accounts get it).
// Returns Selections ready for awsconfig.Generate.
func interactiveTUI(profiles []awsconfig.Profile, accountIDs map[string]string,
	promptLine func(prompt string) (string, bool)) ([]awsconfig.Selection, error) {

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

func newInitCmd() *cobra.Command {
	var (
		force          bool
		toStdout       bool
		verify         bool
		nonInteractive bool
		edit           bool
	)
	c := &cobra.Command{
		Use:   "init",
		Short: "Generate a starter cdkm.yaml from your AWS profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Step 1: parse profiles (+ optional --verify enrichment).
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

			var out []byte

			// Step 2: if --edit, open a prefilled config in $EDITOR.
			if edit {
				sels := allSelections(profiles, accountIDs)
				doc, err := awsconfig.Generate(sels)
				if err != nil {
					return err
				}
				edited, err := editInEditor(doc)
				if err != nil {
					return err
				}
				if err := validateConfigBytes(edited); err != nil {
					if toStdout {
						_, _ = os.Stdout.Write(edited)
						return fmt.Errorf("edited config failed validation: %w", err)
					}
					rejPath := "cdkm.yaml.rej"
					if werr := os.WriteFile(rejPath, edited, 0o644); werr != nil {
						return fmt.Errorf("edited config failed validation: %w (also failed to save recovery file: %v)", err, werr)
					}
					return fmt.Errorf("edited config failed validation: %w\nYour edits were saved to %s — fix and retry", err, rejPath)
				}
				out = edited
			} else {
				// Step 3/4: interactive TUI or non-interactive all-profiles.
				var sels []awsconfig.Selection
				interactive := !nonInteractive && isatty.IsTerminal(os.Stdin.Fd())
				if interactive {
					sels, err = interactiveTUI(profiles, accountIDs, promptLineStdin)
					if err != nil {
						return err
					}
				} else {
					// Non-interactive: include all profiles with no tags/groups.
					choices := make(map[string]profileChoice, len(profiles))
					for _, p := range profiles {
						choices[p.Name] = profileChoice{Include: true}
					}
					sels = buildSelections(profiles, choices, accountIDs)
				}

				if len(sels) == 0 {
					return fmt.Errorf("no profiles selected; nothing to write")
				}
				out, err = awsconfig.Generate(sels)
				if err != nil {
					return err
				}
			}

			// Step 5: output — honor --stdout / --force / write cdkm.yaml.
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
			fmt.Fprintf(os.Stderr, "Wrote %s. Review tags/groups/stacks before use.\n", target)
			return nil
		},
	}
	c.Flags().BoolVar(&force, "force", false, "overwrite an existing cdkm.yaml")
	c.Flags().BoolVar(&toStdout, "stdout", false, "print to stdout instead of writing cdkm.yaml")
	c.Flags().BoolVar(&verify, "verify", false, "run aws sts get-caller-identity per profile to confirm creds and fill account ids")
	c.Flags().BoolVar(&nonInteractive, "non-interactive", false, "include all profiles with empty tags/groups (no prompts)")
	c.Flags().BoolVar(&edit, "edit", false, "open a prefilled config in $VISUAL/$EDITOR/vi for manual editing before writing")
	return c
}
