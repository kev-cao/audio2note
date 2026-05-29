// Package plaudpicker renders an interactive TUI for selecting one or more
// recordings from a user's Plaud library.
package plaudpicker

import (
	"audio2note/internal/plaud"
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
							Bold(true).
							Foreground(lipgloss.Color("15")).
							Border(lipgloss.RoundedBorder()).
							BorderForeground(lipgloss.Color("12")).
							Padding(0, 2)

	selectedPrefixStyle = lipgloss.NewStyle().
											Foreground(lipgloss.Color("12")).
											Bold(true)

	selectedNameStyle = lipgloss.NewStyle().
										Foreground(lipgloss.Color("15")).
										Bold(true)

	pickedStyle = lipgloss.NewStyle().
							Foreground(lipgloss.Color("11")).
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
	// rank is the 1-based position in the user's ordered selection, or 0 if
	// not selected. The model rewrites these whenever the selection changes.
	rank int
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

	cursor := index == m.Index()

	var prefix string
	switch {
	case it.rank > 0 && cursor:
		prefix = pickedStyle.Render(fmt.Sprintf("▶[%d] ", it.rank))
	case it.rank > 0:
		prefix = pickedStyle.Render(fmt.Sprintf(" [%d] ", it.rank))
	case cursor:
		prefix = selectedPrefixStyle.Render("▶    ")
	default:
		prefix = "     "
	}

	name := nameStyle.Render(it.file.Filename)
	if cursor {
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
	list list.Model
	// pickedIDs is the ordered selection (recording IDs in toggle order).
	// Stored separately from items so it survives the list's filter cycles,
	// which rebuild the visible item set.
	pickedIDs []plaud.ID
	// confirmed becomes true when the user hits Enter. If false at exit, the
	// user cancelled (q / ctrl+c) and we return nothing.
	confirmed bool
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		// Only intercept when the user isn't typing into the filter input.
		if m.list.FilterState() != list.Filtering {
			switch msg.String() {
			case " ":
				if it, ok := m.list.SelectedItem().(item); ok {
					m.pickedIDs = toggleID(m.pickedIDs, it.file.ID)
					m.refreshRanks()
				}
				return m, nil
			case "enter":
				// Plain Enter with no explicit picks falls back to "select
				// the current row" so single-file flow stays one keystroke.
				if len(m.pickedIDs) == 0 {
					if it, ok := m.list.SelectedItem().(item); ok {
						m.pickedIDs = []plaud.ID{it.file.ID}
					}
				}
				if len(m.pickedIDs) > 0 {
					m.confirmed = true
					return m, tea.Quit
				}
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string { return m.list.View() }

// refreshRanks rewrites the rank on each item to match its position in
// pickedIDs (1-based) or 0 if not picked. Called whenever the selection
// changes so the delegate renders the right [N] marker.
func (m *model) refreshRanks() {
	ranks := make(map[plaud.ID]int, len(m.pickedIDs))
	for i, id := range m.pickedIDs {
		ranks[id] = i + 1
	}
	items := m.list.Items()
	for i, li := range items {
		it, ok := li.(item)
		if !ok {
			continue
		}
		it.rank = ranks[it.file.ID]
		items[i] = it
	}
	m.list.SetItems(items)
}

// toggleID adds id to ids if absent, or removes it if present.
func toggleID(ids []plaud.ID, id plaud.ID) []plaud.ID {
	for i, existing := range ids {
		if existing == id {
			return append(ids[:i], ids[i+1:]...)
		}
	}
	return append(ids, id)
}

// Pick renders a TUI menu of Plaud files. Space toggles items in/out of an
// ordered selection; Enter confirms. With no explicit picks, Enter selects the
// cursor row (single-file flow). Returns the ordered selection plus ok=true on
// confirmation, or ok=false when the user cancels (q / ctrl+c).
func Pick(files []plaud.File) ([]plaud.File, bool, error) {
	items := make([]list.Item, len(files))
	for i, f := range files {
		items[i] = item{file: f}
	}

	l := list.New(items, delegate{}, 80, 20)
	l.Title = "Pick Plaud recording(s) — space toggles, enter confirms"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle
	l.Styles.TitleBar = l.Styles.TitleBar.PaddingBottom(0)

	final, err := tea.NewProgram(model{list: l}, tea.WithAltScreen()).Run()
	if err != nil {
		return nil, false, err
	}
	fm := final.(model)
	if !fm.confirmed || len(fm.pickedIDs) == 0 {
		return nil, false, nil
	}

	// Map IDs back to files, preserving toggle order.
	byID := make(map[plaud.ID]plaud.File, len(files))
	for _, f := range files {
		byID[f.ID] = f
	}
	out := make([]plaud.File, 0, len(fm.pickedIDs))
	for _, id := range fm.pickedIDs {
		if f, ok := byID[id]; ok {
			out = append(out, f)
		}
	}
	return out, true, nil
}
