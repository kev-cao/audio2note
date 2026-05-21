// Package speakeredit renders the interactive TUI used by --edit-speaker-labels.
//
// Run returns the user's chosen renames (old → new) and a saved bool. If saved
// is false, the user quit without saving and the renames should be ignored.
package speakeredit

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"audio2note/internal/transcriber"
)

const quoteTruncate = 150

type mode int

const (
	browseMode mode = iota
	editMode
	confirmQuitMode
)

type model struct {
	speakers   []transcriber.Speaker
	speakerIdx int
	quoteIdx   int
	renames    map[string]string
	mode       mode
	input      textinput.Model
	saved      bool
}

func newModel(speakers []transcriber.Speaker) model {
	ti := textinput.New()
	ti.CharLimit = 128
	ti.Width = 40
	return model{
		speakers: speakers,
		renames:  map[string]string{},
		input:    ti,
	}
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyMsg)
	switch m.mode {
	case browseMode:
		if !isKey {
			return m, nil
		}
		return m.updateBrowse(keyMsg)
	case editMode:
		if isKey {
			switch keyMsg.String() {
			case "esc":
				m.mode = browseMode
				m.input.Blur()
				return m, nil
			case "enter":
				orig := m.speakers[m.speakerIdx].Name
				v := strings.TrimSpace(m.input.Value())
				if v == "" || v == orig {
					delete(m.renames, orig)
				} else {
					m.renames[orig] = v
				}
				m.mode = browseMode
				m.input.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case confirmQuitMode:
		if !isKey {
			return m, nil
		}
		switch keyMsg.String() {
		case "y", "Y":
			return m, tea.Quit
		case "n", "N", "esc":
			m.mode = browseMode
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.speakerIdx > 0 {
			m.speakerIdx--
			m.quoteIdx = 0
		}
	case "down", "j":
		if m.speakerIdx < len(m.speakers)-1 {
			m.speakerIdx++
			m.quoteIdx = 0
		}
	case "left", "h":
		if m.quoteIdx > 0 {
			m.quoteIdx--
		}
	case "right", "l":
		if m.quoteIdx < len(m.speakers[m.speakerIdx].Quotes)-1 {
			m.quoteIdx++
		}
	case "enter":
		orig := m.speakers[m.speakerIdx].Name
		cur := orig
		if r, ok := m.renames[orig]; ok {
			cur = r
		}
		m.input.SetValue(cur)
		m.input.CursorEnd()
		m.mode = editMode
		return m, m.input.Focus()
	case "ctrl+s":
		m.saved = true
		return m, tea.Quit
	case "q", "ctrl+c":
		if len(m.renames) > 0 {
			m.mode = confirmQuitMode
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m model) View() string {
	if len(m.speakers) == 0 {
		return "(no speakers)\n"
	}
	sp := m.speakers[m.speakerIdx]

	var b strings.Builder
	fmt.Fprintf(&b, "Speaker %d of %d: %s", m.speakerIdx+1, len(m.speakers), sp.Name)
	if newName, ok := m.renames[sp.Name]; ok {
		fmt.Fprintf(&b, "  →  %s", newName)
	}
	b.WriteString("\n")

	if len(sp.Quotes) > 0 {
		fmt.Fprintf(&b, "Quote %d of %d:\n  %q\n", m.quoteIdx+1, len(sp.Quotes), truncate(sp.Quotes[m.quoteIdx].Text, quoteTruncate))
	} else {
		b.WriteString("(no quotes)\n")
	}
	b.WriteString("\n")

	switch m.mode {
	case browseMode:
		b.WriteString("↑/↓ speaker • ←/→ quote • enter rename • ctrl+s save • q quit\n")
	case editMode:
		fmt.Fprintf(&b, "Rename to: %s\n\nenter confirm • esc cancel\n", m.input.View())
	case confirmQuitMode:
		fmt.Fprintf(&b, "You have %d unsaved rename(s). Quit anyway? (y/N)\n", len(m.renames))
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

// Run starts the TUI. Blocks until the user saves or quits.
func Run(speakers []transcriber.Speaker) (map[string]string, bool, error) {
	p := tea.NewProgram(newModel(speakers), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return nil, false, err
	}
	fm := final.(model)
	if !fm.saved {
		return nil, false, nil
	}
	return fm.renames, true, nil
}
