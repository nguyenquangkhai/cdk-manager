package run

import (
	"strings"

	"github.com/nguyenquangkhai/cdk-manager/internal/adapter"
	"github.com/nguyenquangkhai/cdk-manager/internal/target"
)

// BuildCommand constructs an adapter.Command by substituting template variables
// in argv with values from the target (profile, region, account, name) and context.
// argv must contain at least the command name; BuildCommand panics on empty argv.
func BuildCommand(t target.Target, outDir string, argv []string) adapter.Command {
	if len(argv) == 0 {
		panic("run.BuildCommand: argv must contain at least the command name")
	}
	repl := map[string]string{
		"{{profile}}": t.Profile,
		"{{region}}":  t.Region,
		"{{account}}": t.Account,
		"{{target}}":  t.Name,
		"{{outdir}}":  outDir,
	}
	for k, v := range t.Context {
		repl["{{context."+k+"}}"] = v
	}
	subst := func(s string) string {
		for k, v := range repl {
			s = strings.ReplaceAll(s, k, v)
		}
		return s
	}

	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = subst(a)
	}
	return adapter.Command{
		Name: out[0],
		Args: out[1:],
		Env: map[string]string{
			"AWS_PROFILE": t.Profile,
			"AWS_REGION":  t.Region,
		},
	}
}
