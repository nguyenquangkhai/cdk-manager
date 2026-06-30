package ui

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nguyenquangkhai/cdk-manager/internal/engine"
)

// updateMsg wraps an engine.Update for the bubbletea message loop.
type updateMsg struct{ u engine.Update }

// doneMsg signals that all results are available.
type doneMsg struct{ results []engine.Result }

// tuiModel is the bubbletea model for the live progress table.
// It is scaffolded here and wired as the active reporter in Task 10.
type tuiModel struct {
	mu      sync.Mutex
	updates map[string]engine.Update
	done    bool
	results []engine.Result

	headerStyle lipgloss.Style
	rowStyle    lipgloss.Style
}

func newTUIModel() *tuiModel {
	return &tuiModel{
		updates:     make(map[string]engine.Update),
		headerStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")),
		rowStyle:    lipgloss.NewStyle().PaddingLeft(2),
	}
}

func (m *tuiModel) Init() tea.Cmd { return nil }

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case updateMsg:
		m.mu.Lock()
		m.updates[msg.u.Target] = msg.u
		m.mu.Unlock()
	case doneMsg:
		m.done = true
		m.results = msg.results
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *tuiModel) View() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(m.headerStyle.Render("CDK Manager — Live Progress") + "\n\n")

	targets := make([]string, 0, len(m.updates))
	for t := range m.updates {
		targets = append(targets, t)
	}
	sort.Strings(targets)

	for _, t := range targets {
		u := m.updates[t]
		line := u.Line
		if line == "" {
			line = string(u.State)
		}
		sb.WriteString(m.rowStyle.Render(fmt.Sprintf("[%s] %s", t, line)) + "\n")
	}

	if m.done {
		sb.WriteString("\n" + m.headerStyle.Render("Done") + "\n")
		for _, r := range m.results {
			sb.WriteString(m.rowStyle.Render(fmt.Sprintf("[%s] %s", r.Target, r.State)) + "\n")
		}
	}

	return sb.String()
}

// tuiReporter wraps tuiModel and satisfies Reporter.
// It is unused by New() in v1; wired in Task 10 when isTTY is true.
type tuiReporter struct {
	model  *tuiModel
	prog   *tea.Program
	out    io.Writer
}

func newTUIReporter(w io.Writer) *tuiReporter {
	m := newTUIModel()
	p := tea.NewProgram(m, tea.WithOutput(w))
	return &tuiReporter{model: m, prog: p, out: w}
}

func (r *tuiReporter) Update(u engine.Update) {
	r.prog.Send(updateMsg{u: u})
}

func (r *tuiReporter) Done(results []engine.Result) {
	r.prog.Send(doneMsg{results: results})
}
