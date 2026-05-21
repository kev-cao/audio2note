package speakeredit

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"audio2note/internal/transcriber"
)

const quoteTruncate = 150

// tui is the bubbletea adapter. It implements tea.Model and translates input
// events to Model method calls. The Model itself stays bubbletea-free.
type tui struct {
	m     *Model
	input textinput.Model
}

func newTUI(m *Model) tui {
	ti := textinput.New()
	ti.CharLimit = 128
	ti.Width = 40
	return tui{m: m, input: ti}
}

func (t tui) Init() tea.Cmd { return textinput.Blink }

func (t tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	case "ctrl+s":
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
		return "(no speakers)\n"
	}
	sp := t.m.CurrentSpeaker()

	var b strings.Builder
	fmt.Fprintf(&b, "Speaker %d of %d: %s", t.m.SpeakerIndex()+1, len(speakers), sp.Name)
	if newName, ok := t.m.PendingRename(sp.Name); ok {
		fmt.Fprintf(&b, "  →  %s", newName)
	}
	b.WriteString("\n")

	if quote, ok := t.m.CurrentQuote(); ok {
		fmt.Fprintf(&b, "Quote %d of %d:\n  %q\n", t.m.QuoteIndex()+1, len(sp.Quotes), truncate(quote.Text, quoteTruncate))
	} else {
		b.WriteString("(no quotes)\n")
	}
	b.WriteString("\n")

	switch t.m.Mode() {
	case BrowseMode:
		b.WriteString("↑/↓ speaker • ←/→ quote • enter rename • ctrl+s save • q quit\n")
	case EditMode:
		fmt.Fprintf(&b, "Rename to: %s\n\nenter confirm • esc cancel\n", t.input.View())
	case ConfirmQuitMode:
		fmt.Fprintf(&b, "You have %d unsaved rename(s). Quit anyway? (y/N)\n", t.m.PendingRenameCount())
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
