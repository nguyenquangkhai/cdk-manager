package tui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item is one selectable row.
type Item struct {
	Key  string // stable identifier returned on confirm (e.g. profile name)
	Desc string // right-hand description (e.g. "eu-west-1")
}

// Model is a filterable checkbox multiselect. Construct with NewModel,
// run via tea.NewProgram, then read Selected() after it exits.
// Keys: up/down or k/j move; space toggles; "a" toggles all (visible);
// "/" enters filter mode (type to filter, esc/enter leaves filter mode);
// enter confirms; "q"/ctrl+c aborts (sets Aborted()).
type Model struct {
	title     string
	items     []Item
	checked   map[int]bool // index into items
	cursor    int          // index into the visible slice
	filter    string
	filtering bool
	aborted   bool
}

// NewModel creates a new multiselect model.
func NewModel(title string, items []Item, preselectAll bool) Model {
	m := Model{title: title, items: items, checked: map[int]bool{}}
	if preselectAll {
		for i := range items {
			m.checked[i] = true
		}
	}
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// visibleIdx returns item indices passing the current filter, in order.
func (m Model) visibleIdx() []int {
	var out []int
	f := strings.ToLower(m.filter)
	for i, it := range m.items {
		if f == "" || strings.Contains(strings.ToLower(it.Key), f) {
			out = append(out, i)
		}
	}
	return out
}

// visibleKeys returns the Keys currently passing the filter.
func (m Model) visibleKeys() []string {
	idx := m.visibleIdx()
	out := make([]string, len(idx))
	for i, j := range idx {
		out[i] = m.items[j].Key
	}
	return out
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	vis := m.visibleIdx()

	if m.filtering {
		switch km.Type {
		case tea.KeyEnter, tea.KeyEsc:
			m.filtering = false
		case tea.KeyBackspace:
			if n := len(m.filter); n > 0 {
				m.filter = m.filter[:n-1]
			}
		case tea.KeySpace:
			m.filter += " "
		case tea.KeyRunes:
			m.filter += string(km.Runes)
		}
		if m.cursor >= len(m.visibleIdx()) {
			m.cursor = 0
		}
		return m, nil
	}

	switch {
	case km.Type == tea.KeyCtrlC || (km.Type == tea.KeyRunes && string(km.Runes) == "q"):
		m.aborted = true
		return m, tea.Quit
	case km.Type == tea.KeyEnter:
		return m, tea.Quit
	case km.Type == tea.KeyUp || (km.Type == tea.KeyRunes && string(km.Runes) == "k"):
		if m.cursor > 0 {
			m.cursor--
		}
	case km.Type == tea.KeyDown || (km.Type == tea.KeyRunes && string(km.Runes) == "j"):
		if m.cursor < len(vis)-1 {
			m.cursor++
		}
	case km.Type == tea.KeySpace || (km.Type == tea.KeyRunes && string(km.Runes) == " "):
		if m.cursor < len(vis) {
			i := vis[m.cursor]
			m.checked[i] = !m.checked[i]
		}
	case km.Type == tea.KeyRunes && string(km.Runes) == "a":
		// toggle-all over visible: if all visible checked, clear; else set.
		allChecked := len(vis) > 0
		for _, i := range vis {
			if !m.checked[i] {
				allChecked = false
				break
			}
		}
		for _, i := range vis {
			m.checked[i] = !allChecked
		}
	case km.Type == tea.KeyRunes && string(km.Runes) == "/":
		m.filtering = true
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(m.title) + "\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(
		"space toggle · /=filter · a=all · enter=confirm · q=cancel") + "\n")
	if m.filtering || m.filter != "" {
		b.WriteString("filter: " + m.filter + "\n")
	}
	b.WriteString("\n")
	for ci, i := range m.visibleIdx() {
		cursor := "  "
		if ci == m.cursor {
			cursor = "> "
		}
		box := "[ ]"
		if m.checked[i] {
			box = "[x]"
		}
		b.WriteString(cursor + box + " " + m.items[i].Key + "  " +
			lipgloss.NewStyle().Faint(true).Render(m.items[i].Desc) + "\n")
	}
	n := 0
	for _, v := range m.checked {
		if v {
			n++
		}
	}
	b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(
		strings.TrimSpace(strconv.Itoa(n)+" selected")) + "\n")
	return b.String()
}

// Selected returns the Keys of checked items, in original item order.
func (m Model) Selected() []string {
	var out []string
	for i := range m.items {
		if m.checked[i] {
			out = append(out, m.items[i].Key)
		}
	}
	return out
}

// Aborted reports whether the user cancelled (q/ctrl+c) rather than confirming.
func (m Model) Aborted() bool { return m.aborted }
