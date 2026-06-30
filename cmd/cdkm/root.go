package main

import (
	"context"
	"os"
	"time"

	"github.com/mattn/go-isatty"
	ver "github.com/nguyenquangkhai/cdk-manager/internal/version"
	"github.com/spf13/cobra"
)

const versionCachePath = ".cdkm/version-check.json"

var rootCmd *cobra.Command

func init() {
	rootCmd = &cobra.Command{
		Use:     "cdkm",
		Short:   "CDK Manager — deploy/destroy/diff/synth across multiple AWS accounts",
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if autoCheckAllowed() {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					_ = ver.Refresh(ctx, versionCachePath, ver.GitHubFetcher, 24*time.Hour)
				}()
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if cmd.Name() == "version" {
				return
			}
			if autoCheckAllowed() {
				ver.WarnIfOutdated(os.Stderr, versionCachePath, version)
			}
		},
	}

	// Persistent flags (available to all subcommands)
	pf := rootCmd.PersistentFlags()
	pf.String("config", "cdkm.yaml", "Path to config file")

	// Selector flags (exactly one must be set)
	pf.Bool("all", false, "Target all accounts")
	pf.String("group", "", "Target a named group")
	pf.String("account", "", "Target a single account by name")
	pf.String("tag", "", "Target accounts matching a tag")

	// Run flags
	pf.Int("concurrency", 0, "Max parallel jobs (0 = use config default)")
	pf.Bool("dry-run", false, "Print commands without executing")
	pf.Bool("fail-fast", false, "Abort remaining jobs on first failure")
	pf.String("require-approval", "", "CDK require-approval level (never/any-change/broadening)")

	// Register subcommands
	addDeployCmd(rootCmd)
	addDestroyCmd(rootCmd)
	addDiffCmd(rootCmd)
	addSynthCmd(rootCmd)
	addRunCmd(rootCmd)
	addListCmd(rootCmd)
	addStatusCmd(rootCmd)
	rootCmd.AddCommand(newVersionCmd())
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// isTerminal reports whether stdout is a TTY.
func isTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
}

// autoCheckAllowed returns true only when an automatic update check is
// appropriate: a real release build, no opt-out env var, and a TTY.
func autoCheckAllowed() bool {
	if version == "dev" {
		return false
	}
	if os.Getenv("CDKM_NO_UPDATE_CHECK") != "" {
		return false
	}
	return isTerminal()
}
