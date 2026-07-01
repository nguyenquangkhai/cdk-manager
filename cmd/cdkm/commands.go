package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/adapter/cdk"
	"github.com/nguyenquangkhai/cdk-manager/internal/config"
	"github.com/nguyenquangkhai/cdk-manager/internal/engine"
	"github.com/nguyenquangkhai/cdk-manager/internal/report"
	"github.com/nguyenquangkhai/cdk-manager/internal/run"
	"github.com/nguyenquangkhai/cdk-manager/internal/safety"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
	"github.com/nguyenquangkhai/cdk-manager/internal/ui"
	ver "github.com/nguyenquangkhai/cdk-manager/internal/version"
	"github.com/spf13/cobra"
)

// buildCDKJobs assembles engine.Job slice from resolved targets for a CDK operation.
func buildCDKJobs(c *config.Config, tgts []target.Target, op adapter.Operation, cliStacks []string, requireApproval string) []engine.Job {
	a := cdk.New()
	jobs := make([]engine.Job, 0, len(tgts))
	for _, t := range tgts {
		stacks := cliStacks
		if len(stacks) == 0 {
			stacks = target.Stacks(c, t)
		}
		jobs = append(jobs, engine.Job{Target: t, Command: a.Build(t, op, stacks, requireApproval)})
	}
	return jobs
}

// selectorFromFlags returns a target.Selector from the persistent selector flags.
func selectorFromFlags(all bool, group, account, tag string) (target.Selector, error) {
	n := 0
	if all {
		n++
	}
	if group != "" {
		n++
	}
	if account != "" {
		n++
	}
	if tag != "" {
		n++
	}
	if n != 1 {
		return target.Selector{}, fmt.Errorf("exactly one of --all/--group/--account/--tag must be set (got %d)", n)
	}
	return target.Selector{All: all, Group: group, Account: account, Tag: tag}, nil
}

// selectorLabel returns a human-readable label for the selector (used in safety confirmation).
func selectorLabel(sel target.Selector) string {
	switch {
	case sel.All:
		return "all"
	case sel.Group != "":
		return sel.Group
	case sel.Account != "":
		return sel.Account
	case sel.Tag != "":
		return sel.Tag
	default:
		return "unknown"
	}
}

// runCDKOp is the shared body for deploy/destroy/diff/synth subcommands.
func runCDKOp(
	op adapter.Operation,
	configPath string,
	all bool, group, account, tag string,
	concurrency int, dryRun, failFast bool, requireApprovalFlag string,
	cliStacks []string,
) error {
	cfg, err := config.LoadLayered(config.GlobalConfigPath(), configPath)
	if err != nil {
		return err
	}

	sel, err := selectorFromFlags(all, group, account, tag)
	if err != nil {
		return err
	}

	tgts, err := target.Resolve(cfg, sel)
	if err != nil {
		return err
	}

	if op == adapter.OpDestroy {
		label := selectorLabel(sel)
		if err := safety.ConfirmDestroy(os.Stdin, os.Stdout, op, label, tgts); err != nil {
			return err
		}
	}

	// Determine effective require-approval: CLI flag > config default
	reqApproval := requireApprovalFlag
	if reqApproval == "" {
		reqApproval = cfg.Defaults.RequireApproval
	}

	// Determine effective concurrency: CLI flag > config default
	conc := concurrency
	if conc == 0 {
		conc = cfg.Defaults.Concurrency
	}

	jobs := buildCDKJobs(cfg, tgts, op, cliStacks, reqApproval)

	isTTY := isTerminal()
	rep := ui.New(os.Stdout, isTTY)

	a := cdk.New()
	results := engine.Run(context.Background(), jobs, engine.Options{
		Concurrency: conc,
		FailFast:    failFast,
		DryRun:      dryRun,
		LogDir:      ".cdkm/logs",
		Parse:       a.ParseStatus,
		OnUpdate:    rep.Update,
	})
	rep.Done(results)
	_ = report.SaveState(".cdkm/state.json", results)
	os.Exit(report.Summarize(os.Stdout, results))
	return nil
}

func addDeployCmd(root *cobra.Command) {
	var cliStacks []string
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy CDK stacks to target accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			f := root.PersistentFlags()
			configPath, _ := f.GetString("config")
			all, _ := f.GetBool("all")
			group, _ := f.GetString("group")
			account, _ := f.GetString("account")
			tag, _ := f.GetString("tag")
			concurrency, _ := f.GetInt("concurrency")
			dryRun, _ := f.GetBool("dry-run")
			failFast, _ := f.GetBool("fail-fast")
			requireApproval, _ := f.GetString("require-approval")
			return runCDKOp(adapter.OpDeploy, configPath, all, group, account, tag, concurrency, dryRun, failFast, requireApproval, cliStacks)
		},
	}
	cmd.Flags().StringSliceVar(&cliStacks, "stacks", nil, "Override stacks to deploy (comma-separated)")
	root.AddCommand(cmd)
}

func addDestroyCmd(root *cobra.Command) {
	var cliStacks []string
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroy CDK stacks in target accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			f := root.PersistentFlags()
			configPath, _ := f.GetString("config")
			all, _ := f.GetBool("all")
			group, _ := f.GetString("group")
			account, _ := f.GetString("account")
			tag, _ := f.GetString("tag")
			concurrency, _ := f.GetInt("concurrency")
			dryRun, _ := f.GetBool("dry-run")
			failFast, _ := f.GetBool("fail-fast")
			requireApproval, _ := f.GetString("require-approval")
			return runCDKOp(adapter.OpDestroy, configPath, all, group, account, tag, concurrency, dryRun, failFast, requireApproval, cliStacks)
		},
	}
	cmd.Flags().StringSliceVar(&cliStacks, "stacks", nil, "Override stacks to destroy (comma-separated)")
	root.AddCommand(cmd)
}

func addDiffCmd(root *cobra.Command) {
	var cliStacks []string
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show diff of CDK stacks",
		RunE: func(cmd *cobra.Command, args []string) error {
			f := root.PersistentFlags()
			configPath, _ := f.GetString("config")
			all, _ := f.GetBool("all")
			group, _ := f.GetString("group")
			account, _ := f.GetString("account")
			tag, _ := f.GetString("tag")
			concurrency, _ := f.GetInt("concurrency")
			dryRun, _ := f.GetBool("dry-run")
			failFast, _ := f.GetBool("fail-fast")
			requireApproval, _ := f.GetString("require-approval")
			return runCDKOp(adapter.OpDiff, configPath, all, group, account, tag, concurrency, dryRun, failFast, requireApproval, cliStacks)
		},
	}
	cmd.Flags().StringSliceVar(&cliStacks, "stacks", nil, "Override stacks to diff (comma-separated)")
	root.AddCommand(cmd)
}

func addSynthCmd(root *cobra.Command) {
	var cliStacks []string
	cmd := &cobra.Command{
		Use:   "synth",
		Short: "Synthesize CDK stacks",
		RunE: func(cmd *cobra.Command, args []string) error {
			f := root.PersistentFlags()
			configPath, _ := f.GetString("config")
			all, _ := f.GetBool("all")
			group, _ := f.GetString("group")
			account, _ := f.GetString("account")
			tag, _ := f.GetString("tag")
			concurrency, _ := f.GetInt("concurrency")
			dryRun, _ := f.GetBool("dry-run")
			failFast, _ := f.GetBool("fail-fast")
			requireApproval, _ := f.GetString("require-approval")
			return runCDKOp(adapter.OpSynth, configPath, all, group, account, tag, concurrency, dryRun, failFast, requireApproval, cliStacks)
		},
	}
	cmd.Flags().StringSliceVar(&cliStacks, "stacks", nil, "Override stacks to synth (comma-separated)")
	root.AddCommand(cmd)
}

func addRunCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:                "run -- <command> [args...]",
		Short:              "Run an arbitrary command against each target",
		DisableFlagParsing: false,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("run requires a command after '--'; e.g.: cdkm run --group dev -- aws sts get-caller-identity")
			}

			f := root.PersistentFlags()
			configPath, _ := f.GetString("config")
			all, _ := f.GetBool("all")
			group, _ := f.GetString("group")
			account, _ := f.GetString("account")
			tag, _ := f.GetString("tag")
			concurrency, _ := f.GetInt("concurrency")
			dryRun, _ := f.GetBool("dry-run")
			failFast, _ := f.GetBool("fail-fast")

			cfg, err := config.LoadLayered(config.GlobalConfigPath(), configPath)
			if err != nil {
				return err
			}
			sel, err := selectorFromFlags(all, group, account, tag)
			if err != nil {
				return err
			}
			tgts, err := target.Resolve(cfg, sel)
			if err != nil {
				return err
			}

			conc := concurrency
			if conc == 0 {
				conc = cfg.Defaults.Concurrency
			}

			jobs := make([]engine.Job, 0, len(tgts))
			for _, t := range tgts {
				outDir := ".cdkm/out/" + t.Name
				jobs = append(jobs, engine.Job{
					Target:  t,
					Command: run.BuildCommand(t, outDir, args),
				})
			}

			isTTY := isTerminal()
			rep := ui.New(os.Stdout, isTTY)

			results := engine.Run(context.Background(), jobs, engine.Options{
				Concurrency: conc,
				FailFast:    failFast,
				DryRun:      dryRun,
				LogDir:      ".cdkm/logs",
				Parse:       nil,
				OnUpdate:    rep.Update,
			})
			rep.Done(results)
			_ = report.SaveState(".cdkm/state.json", results)
			os.Exit(report.Summarize(os.Stdout, results))
			return nil
		},
	}
	root.AddCommand(cmd)
}

func addListCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resolved targets and effective stacks",
		RunE: func(cmd *cobra.Command, args []string) error {
			f := root.PersistentFlags()
			configPath, _ := f.GetString("config")
			all, _ := f.GetBool("all")
			group, _ := f.GetString("group")
			account, _ := f.GetString("account")
			tag, _ := f.GetString("tag")

			cfg, err := config.LoadLayered(config.GlobalConfigPath(), configPath)
			if err != nil {
				return err
			}
			sel, err := selectorFromFlags(all, group, account, tag)
			if err != nil {
				return err
			}
			tgts, err := target.Resolve(cfg, sel)
			if err != nil {
				return err
			}
			for _, t := range tgts {
				stacks := target.Stacks(cfg, t)
				fmt.Printf("%s\t%s/%s\tstacks=%v\n", t.Name, t.Profile, t.Region, stacks)
			}
			return nil
		},
	}
	root.AddCommand(cmd)
}

func addStatusCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show last run state from .cdkm/state.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(".cdkm/state.json")
			if err != nil {
				return fmt.Errorf("no state found (run a command first): %w", err)
			}
			var results []engine.Result
			if err := json.Unmarshal(b, &results); err != nil {
				return fmt.Errorf("parse state: %w", err)
			}
			code := report.Summarize(os.Stdout, results)
			os.Exit(code)
			return nil
		},
	}
	root.AddCommand(cmd)
}

func newVersionCmd() *cobra.Command {
	var check bool
	c := &cobra.Command{
		Use:   "version",
		Short: "Print cdkm version (and optionally check for updates)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !check {
				fmt.Printf("cdkm %s\n", version)
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
			defer cancel()
			_, err := ver.CheckNow(ctx, os.Stdout, versionCachePath, version, ver.GitHubFetcher)
			if err != nil {
				fmt.Fprintf(os.Stderr, "update check failed: %v\n", err)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&check, "check", false, "query GitHub for the latest release")
	return c
}
