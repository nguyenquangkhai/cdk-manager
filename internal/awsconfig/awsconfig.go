package awsconfig

import (
	"os"
	"sort"
	"strings"

	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

type Profile struct {
	Name      string
	Region    string
	AccountID string
}

type Selection struct {
	Name      string
	Profile   string
	Region    string
	AccountID string
	Tags      []string
	Groups    []string
}

func Parse(configPath, credsPath string) ([]Profile, error) {
	byName := map[string]*Profile{}

	if f, err := loadINI(configPath); err == nil {
		for _, sec := range f.Sections() {
			name := sectionToProfileName(sec.Name())
			if name == "" {
				continue
			}
			p := ensure(byName, name)
			if v := sec.Key("region").String(); v != "" {
				p.Region = v
			} else if v := sec.Key("sso_region").String(); v != "" && p.Region == "" {
				p.Region = v
			}
			if v := sec.Key("sso_account_id").String(); v != "" {
				p.AccountID = v
			}
		}
	}
	if f, err := loadINI(credsPath); err == nil {
		for _, sec := range f.Sections() {
			name := sec.Name()
			if name == ini.DefaultSection {
				// go-ini's implicit default section; skip unless it has keys.
				if len(sec.Keys()) == 0 {
					continue
				}
				name = "default"
			}
			if name == "" {
				continue
			}
			ensure(byName, name) // include credentials-only profiles
		}
	}

	out := make([]Profile, 0, len(byName))
	for _, p := range byName {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func loadINI(path string) (*ini.File, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return ini.Load(path)
}

func ensure(m map[string]*Profile, name string) *Profile {
	if p, ok := m[name]; ok {
		return p
	}
	p := &Profile{Name: name}
	m[name] = p
	return p
}

// sectionToProfileName maps an ~/.aws/config section name to a profile name,
// or "" if the section is not a profile (sso-session, services, etc.).
func sectionToProfileName(section string) string {
	switch {
	case section == "default":
		return "default"
	case strings.HasPrefix(section, "profile "):
		return strings.TrimSpace(strings.TrimPrefix(section, "profile "))
	default:
		return "" // sso-session, services, DEFAULT, etc.
	}
}

func Generate(sels []Selection) ([]byte, error) {
	type account struct {
		Profile string            `yaml:"profile"`
		Region  string            `yaml:"region"`
		Tags    []string          `yaml:"tags"`
		Context map[string]string `yaml:"context,omitempty"`
	}
	type group struct {
		Accounts []string `yaml:"accounts"`
	}
	doc := struct {
		Accounts map[string]account `yaml:"accounts"`
		Groups   map[string]group   `yaml:"groups"`
		Stacks   struct {
			Shared []string `yaml:"shared"`
		} `yaml:"stacks"`
	}{
		Accounts: map[string]account{},
		Groups:   map[string]group{},
	}

	for _, s := range sels {
		a := account{Profile: s.Profile, Region: s.Region, Tags: s.Tags}
		if a.Tags == nil {
			a.Tags = []string{}
		}
		if s.AccountID != "" {
			a.Context = map[string]string{"account": s.AccountID}
		}
		doc.Accounts[s.Name] = a
		for _, g := range s.Groups {
			gr := doc.Groups[g]
			gr.Accounts = append(gr.Accounts, s.Name)
			doc.Groups[g] = gr
		}
	}
	for name, gr := range doc.Groups {
		sort.Strings(gr.Accounts)
		doc.Groups[name] = gr
	}
	doc.Stacks.Shared = []string{}

	return yaml.Marshal(doc)
}
