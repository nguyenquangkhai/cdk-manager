package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func keys(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func items() []Item {
	return []Item{
		{Key: "prod-eu", Desc: "eu-west-1"},
		{Key: "prod-us", Desc: "us-east-1"},
		{Key: "dev-eu", Desc: "eu-west-1"},
	}
}

func TestToggleAndSelect(t *testing.T) {
	m := NewModel("Select", items(), false)
	// move to first item already at 0; toggle it.
	mm, _ := m.Update(keys(" "))
	m = mm.(Model)
	// down to index 2 (dev-eu) and toggle.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = mm.(Model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = mm.(Model)
	mm, _ = m.Update(keys(" "))
	m = mm.(Model)

	got := m.Selected()
	if len(got) != 2 || got[0] != "prod-eu" || got[1] != "dev-eu" {
		t.Fatalf("Selected()=%v want [prod-eu dev-eu] in item order", got)
	}
}

func TestPreselectAllAndToggleAll(t *testing.T) {
	m := NewModel("Select", items(), true)
	if len(m.Selected()) != 3 {
		t.Fatalf("preselectAll should check all, got %v", m.Selected())
	}
	// "a" toggles all visible off (all currently checked -> uncheck).
	mm, _ := m.Update(keys("a"))
	m = mm.(Model)
	if len(m.Selected()) != 0 {
		t.Fatalf("toggle-all should clear, got %v", m.Selected())
	}
}

func TestFilterLimitsToggleAllAndVisible(t *testing.T) {
	m := NewModel("Select", items(), false)
	// enter filter mode and type "prod"
	mm, _ := m.Update(keys("/"))
	m = mm.(Model)
	for _, r := range "prod" {
		mm, _ = m.Update(keys(string(r)))
		m = mm.(Model)
	}
	if vk := m.visibleKeys(); len(vk) != 2 || vk[0] != "prod-eu" || vk[1] != "prod-us" {
		t.Fatalf("visibleKeys=%v want [prod-eu prod-us]", vk)
	}
	// leave filter mode, toggle-all (affects only visible prod-*)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	mm, _ = m.Update(keys("a"))
	m = mm.(Model)
	got := m.Selected()
	if len(got) != 2 || got[0] != "prod-eu" || got[1] != "prod-us" {
		t.Fatalf("toggle-all under filter should select only prod-*, got %v", got)
	}
}

func TestAbort(t *testing.T) {
	m := NewModel("Select", items(), false)
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = mm.(Model)
	if !m.Aborted() {
		t.Fatal("ctrl+c should set Aborted")
	}
}
