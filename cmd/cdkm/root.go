package main

import (
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var rootCmd *cobra.Command

func init() {
	rootCmd = &cobra.Command{
		Use:     "cdkm",
		Short:   "CDK Manager — deploy/destroy/diff/synth across multiple AWS accounts",
		Version: version,
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
