// Package plaudpicker renders an interactive TUI for selecting a recording
// from a user's Plaud library.
package plaudpicker

import (
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"audio2note/internal/plaud"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 0, 1, 2)

	selectedPrefixStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("12")).
				Bold(true)

	selectedNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Bold(true)

	nameStyle = lipgloss.NewStyle()

	idStyle = lipgloss.NewStyle().
		Faint(true)

	durationStyle = lipgloss.NewStyle().
			Faint(true).
			Italic(true)

	hasTranscriptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10")).
				Faint(true)
)

type item struct {
	file plaud.File
}

func (i item) FilterValue() string { return i.file.Filename }

type delegate struct{}

func (delegate) Height() int                             { return 1 }
func (delegate) Spacing() int                            { return 0 }
func (delegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (delegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(item)
	if !ok {
		return
	}

	selected := index == m.Index()

	prefix := "  "
	name := nameStyle.Render(it.file.Filename)
	if selected {
		prefix = selectedPrefixStyle.Render("▶ ")
		name = selectedNameStyle.Render(it.file.Filename)
	}

	idShort := string(it.file.ID)
	if len(idShort) > 8 {
		idShort = idShort[:8] + "…"
	}

	dur := it.file.Duration.Round(time.Second).String()

	line := fmt.Sprintf("%s%s  %s  %s",
		prefix,
		name,
		idStyle.Render("("+idShort+")"),
		durationStyle.Render(dur),
	)
	if it.file.HasTranscript {
		line += "  " + hasTranscriptStyle.Render("✓ transcript")
	}
	fmt.Fprint(w, line)
}

type model struct {
	list   list.Model
	chosen *plaud.File
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		// Intercept Enter as "select" — but only when not actively typing
		// into the list's filter input, where Enter means "apply filter".
		if m.list.FilterState() != list.Filtering && msg.String() == "enter" {
			if it, ok := m.list.SelectedItem().(item); ok {
				m.chosen = &it.file
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string { return m.list.View() }

// Pick renders a TUI menu of Plaud files. Returns the selected file plus
// ok=true on selection, or ok=false when the user cancels (q / ctrl+c).
func Pick(files []plaud.File) (plaud.File, bool, error) {
	items := make([]list.Item, len(files))
	for i, f := range files {
		items[i] = item{file: f}
	}

	l := list.New(items, delegate{}, 80, 20)
	l.Title = "Pick a Plaud recording"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	final, err := tea.NewProgram(model{list: l}, tea.WithAltScreen()).Run()
	if err != nil {
		return plaud.File{}, false, err
	}
	fm := final.(model)
	if fm.chosen == nil {
		return plaud.File{}, false, nil
	}
	return *fm.chosen, true, nil
}
