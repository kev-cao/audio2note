package speakeredit

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"audio2note/internal/transcriber"
)

// quoteTruncate caps absurdly long quotes after wrapping kicks in; an
// individual quote rendered in the TUI shouldn't dominate the whole screen.
const quoteTruncate = 512

// defaultWidth is used before bubbletea sends the first WindowSizeMsg.
const defaultWidth = 80

var (
	labelStyle = lipgloss.NewStyle().
			Faint(true).
			Italic(true)

	speakerNameStyle = lipgloss.NewStyle().
				Bold(true)

	renameArrowStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10")) // bright green

	renameNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true)

	quoteStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("8")). // gray
			PaddingLeft(2).
			Italic(true)

	helpStyle = lipgloss.NewStyle().
			Faint(true)

	promptStyle = lipgloss.NewStyle().
			Bold(true)

	confirmStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")). // bright yellow
			Bold(true)
)

// tui is the bubbletea adapter. It implements tea.Model and translates input
// events to Model method calls. The Model itself stays bubbletea-free.
type tui struct {
	m     *Model
	input textinput.Model
	width int
}

func newTUI(m *Model) tui {
	ti := textinput.New()
	ti.CharLimit = 128
	ti.Width = 40
	return tui{m: m, input: ti, width: defaultWidth}
}

func (t tui) Init() tea.Cmd { return textinput.Blink }

func (t tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		t.width = ws.Width
		return t, nil
	}
	switch t.m.Mode() {
	case BrowseMode:
		key, ok := msg.(tea.KeyMsg)
		if !ok {
			return t, nil
		}
		return t.updateBrowse(key)
	case EditMode:
		return t.updateEdit(msg)
	case ConfirmQuitMode:
		key, ok := msg.(tea.KeyMsg)
		if !ok {
			return t, nil
		}
		return t.updateConfirm(key)
	}
	return t, nil
}

func (t tui) updateBrowse(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "down", "j":
		t.m.NextSpeaker()
	case "up", "k":
		t.m.PrevSpeaker()
	case "right", "l":
		t.m.NextQuote()
	case "left", "h":
		t.m.PrevQuote()
	case "enter":
		orig := t.m.CurrentSpeaker().Name
		if pending, ok := t.m.PendingRename(orig); ok {
			t.input.SetValue(pending)
		} else {
			t.input.SetValue(orig)
		}
		t.input.CursorEnd()
		t.m.BeginEdit()
		return t, t.input.Focus()
	case "s":
		t.m.Save()
		return t, tea.Quit
	case "q", "ctrl+c":
		if t.m.RequestQuit() == QuitNow {
			return t, tea.Quit
		}
	}
	return t, nil
}

func (t tui) updateEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			t.m.CancelEdit()
			t.input.Blur()
			return t, nil
		case "enter":
			t.m.CommitEdit(t.input.Value())
			t.input.Blur()
			return t, nil
		}
	}
	var cmd tea.Cmd
	t.input, cmd = t.input.Update(msg)
	return t, cmd
}

func (t tui) updateConfirm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "y", "Y":
		t.m.ConfirmQuit(true)
		return t, tea.Quit
	case "n", "N", "esc":
		t.m.ConfirmQuit(false)
	}
	return t, nil
}

func (t tui) View() string {
	speakers := t.m.Speakers()
	if len(speakers) == 0 {
		return labelStyle.Render("(no speakers)") + "\n"
	}
	sp := t.m.CurrentSpeaker()

	var b strings.Builder

	// Header: "Speaker X of Y" (faint) + bolded name + optional "→ NewName"
	fmt.Fprintf(&b, "%s  %s",
		labelStyle.Render(fmt.Sprintf("Speaker %d of %d", t.m.SpeakerIndex()+1, len(speakers))),
		speakerNameStyle.Render(sp.Name),
	)
	if newName, ok := t.m.PendingRename(sp.Name); ok {
		fmt.Fprintf(&b, "  %s  %s",
			renameArrowStyle.Render("→"),
			renameNameStyle.Render(newName),
		)
	}
	b.WriteString("\n\n")

	// Quote block — width-aware so long quotes wrap instead of overflowing.
	if quote, ok := t.m.CurrentQuote(); ok {
		quoteWidth := max(t.width-4, 20) // leave room for border + padding
		b.WriteString(quoteStyle.Width(quoteWidth).Render(truncate(quote.Text, quoteTruncate)))
		b.WriteString("\n")
		b.WriteString(labelStyle.Render(fmt.Sprintf("quote %d of %d", t.m.QuoteIndex()+1, len(sp.Quotes))))
		b.WriteString("\n")
	} else {
		b.WriteString(labelStyle.Render("(no quotes)"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Mode-specific bottom section
	switch t.m.Mode() {
	case BrowseMode:
		b.WriteString(helpStyle.Render("↑/↓ speaker  •  ←/→ quote  •  enter rename  •  s save  •  q quit"))
		b.WriteString("\n")
	case EditMode:
		fmt.Fprintf(&b, "%s %s\n\n%s\n",
			promptStyle.Render("Rename to:"),
			t.input.View(),
			helpStyle.Render("enter confirm  •  esc cancel"),
		)
	case ConfirmQuitMode:
		fmt.Fprintf(&b, "%s\n\n%s\n",
			confirmStyle.Render(fmt.Sprintf("You have %d unsaved rename(s). Quit anyway?", t.m.PendingRenameCount())),
			helpStyle.Render("y  discard and quit  •  n / esc  keep editing"),
		)
	}
	return b.String()
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// Run starts the interactive editor. Blocks until the user saves or quits.
// Returns the chosen renames and a saved bool; if saved is false, the renames
// slice should be ignored.
func Run(speakers []transcriber.Speaker) (map[string]string, bool, error) {
	m := NewModel(speakers)
	if _, err := tea.NewProgram(newTUI(m), tea.WithAltScreen()).Run(); err != nil {
		return nil, false, err
	}
	if !m.Saved() {
		return nil, false, nil
	}
	return m.Renames(), true, nil
}
