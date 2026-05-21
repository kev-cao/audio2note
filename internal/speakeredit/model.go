// Package speakeredit owns the interactive speaker-rename feature.
//
// The package is split into two concerns:
//   - Model (this file): the pure-Go domain model with named operations and
//     queries. No bubbletea or bubbles imports. Driven directly by tests.
//   - tui (tui.go): the bubbletea adapter that owns the terminal-side widgets
//     and translates input events to Model method calls.
package speakeredit

import (
	"strings"

	"audio2note/internal/transcriber"
)

// Mode is the high-level interaction state of the editor.
type Mode int

const (
	BrowseMode Mode = iota
	EditMode
	ConfirmQuitMode
)

// QuitOutcome is returned by RequestQuit to tell the caller whether the user
// can exit immediately or whether confirmation is required first.
type QuitOutcome int

const (
	QuitNow QuitOutcome = iota
	QuitNeedsConfirm
)

// Model holds editor state and exposes named operations on it. It has no
// knowledge of bubbletea, bubbles, or the terminal.
type Model struct {
	speakers   []transcriber.Speaker
	speakerIdx int
	quoteIdx   int
	renames    map[string]string
	mode       Mode
	saved      bool
	shouldExit bool
}

// NewModel creates a Model positioned at the first speaker's first quote.
func NewModel(speakers []transcriber.Speaker) *Model {
	return &Model{
		speakers: speakers,
		renames:  map[string]string{},
	}
}

// --- navigation (no-ops at clamps) ---

func (m *Model) NextSpeaker() {
	if m.speakerIdx < len(m.speakers)-1 {
		m.speakerIdx++
		m.quoteIdx = 0
	}
}

func (m *Model) PrevSpeaker() {
	if m.speakerIdx > 0 {
		m.speakerIdx--
		m.quoteIdx = 0
	}
}

func (m *Model) NextQuote() {
	sp := m.speakers[m.speakerIdx]
	if m.quoteIdx < len(sp.Quotes)-1 {
		m.quoteIdx++
	}
}

func (m *Model) PrevQuote() {
	if m.quoteIdx > 0 {
		m.quoteIdx--
	}
}

// --- mode transitions ---

// BeginEdit moves to EditMode. The view layer is responsible for showing an
// input widget pre-filled with the current speaker's name (or any pending
// rename, queried via PendingRename).
func (m *Model) BeginEdit() {
	m.mode = EditMode
}

// CommitEdit records a rename for the current speaker and returns to
// BrowseMode. An empty (after trim) or unchanged value clears any pending
// rename for the speaker.
func (m *Model) CommitEdit(newName string) {
	orig := m.speakers[m.speakerIdx].Name
	v := strings.TrimSpace(newName)
	if v == "" || v == orig {
		delete(m.renames, orig)
	} else {
		m.renames[orig] = v
	}
	m.mode = BrowseMode
}

// CancelEdit returns to BrowseMode without modifying renames.
func (m *Model) CancelEdit() {
	m.mode = BrowseMode
}

// RequestQuit handles a user-initiated quit. If there are pending renames it
// switches to ConfirmQuitMode and returns QuitNeedsConfirm; otherwise it marks
// the model as ready to exit and returns QuitNow.
func (m *Model) RequestQuit() QuitOutcome {
	if len(m.renames) > 0 {
		m.mode = ConfirmQuitMode
		return QuitNeedsConfirm
	}
	m.shouldExit = true
	return QuitNow
}

// ConfirmQuit resolves a ConfirmQuitMode prompt. confirmed=true causes the
// editor to exit (saved stays false — renames are discarded). confirmed=false
// returns to BrowseMode preserving the pending renames.
func (m *Model) ConfirmQuit(confirmed bool) {
	if confirmed {
		m.shouldExit = true
		return
	}
	m.mode = BrowseMode
}

// Save marks the model as saved and ready to exit. The view layer should then
// terminate the program; the caller of Run reads Renames() to apply them.
func (m *Model) Save() {
	m.saved = true
	m.shouldExit = true
}

// --- queries ---

func (m *Model) Mode() Mode                      { return m.mode }
func (m *Model) Speakers() []transcriber.Speaker { return m.speakers }
func (m *Model) SpeakerIndex() int               { return m.speakerIdx }
func (m *Model) QuoteIndex() int                 { return m.quoteIdx }

// CurrentSpeaker returns the speaker currently under the cursor. Panics on an
// empty Model — callers should check len(Speakers()) > 0.
func (m *Model) CurrentSpeaker() transcriber.Speaker {
	return m.speakers[m.speakerIdx]
}

// CurrentQuote returns the quote at the current quoteIdx, or false if the
// current speaker has no quotes.
func (m *Model) CurrentQuote() (transcriber.Quote, bool) {
	sp := m.speakers[m.speakerIdx]
	if m.quoteIdx >= len(sp.Quotes) {
		return transcriber.Quote{}, false
	}
	return sp.Quotes[m.quoteIdx], true
}

// PendingRename returns the new name a speaker has been renamed to, if any.
func (m *Model) PendingRename(originalName string) (string, bool) {
	v, ok := m.renames[originalName]
	return v, ok
}

func (m *Model) PendingRenameCount() int    { return len(m.renames) }
func (m *Model) Renames() map[string]string { return m.renames }
func (m *Model) Saved() bool                { return m.saved }
func (m *Model) ShouldExit() bool           { return m.shouldExit }
