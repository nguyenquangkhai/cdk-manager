package target

import (
	"fmt"
	"sort"

	"github.com/nguyenquangkhai/cdk-manager/internal/config"
)

type Target struct {
	Name    string
	Profile string
	Region  string
	Account string
	Context map[string]string
}

type Selector struct {
	All     bool
	Group   string
	Account string
	Tag     string
}

func newTarget(name string, a config.Account) Target {
	return Target{
		Name:    name,
		Profile: a.Profile,
		Region:  a.Region,
		Account: a.Context["account"],
		Context: a.Context,
	}
}

func Resolve(c *config.Config, s Selector) ([]Target, error) {
	names := map[string]struct{}{}
	switch {
	case s.All:
		for n := range c.Accounts {
			names[n] = struct{}{}
		}
	case s.Account != "":
		if _, ok := c.Accounts[s.Account]; !ok {
			return nil, fmt.Errorf("unknown account %q", s.Account)
		}
		names[s.Account] = struct{}{}
	case s.Tag != "":
		for n, a := range c.Accounts {
			for _, t := range a.Tags {
				if t == s.Tag {
					names[n] = struct{}{}
				}
			}
		}
	case s.Group != "":
		g, ok := c.Groups[s.Group]
		if !ok {
			return nil, fmt.Errorf("unknown group %q", s.Group)
		}
		for _, n := range g.Accounts {
			names[n] = struct{}{}
		}
		for n, a := range c.Accounts {
			for _, gt := range g.Tags {
				for _, at := range a.Tags {
					if gt == at {
						names[n] = struct{}{}
					}
				}
			}
		}
	default:
		return nil, fmt.Errorf("no selector provided (use --all/--group/--account/--tag)")
	}

	if len(names) == 0 {
		return nil, fmt.Errorf("selector matched zero accounts")
	}
	out := make([]Target, 0, len(names))
	for n := range names {
		out = append(out, newTarget(n, c.Accounts[n]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func Stacks(c *config.Config, t Target) []string {
	out := append([]string{}, c.Stacks.Shared...)
	out = append(out, c.Stacks.PerAccount[t.Name]...)
	return out
}
