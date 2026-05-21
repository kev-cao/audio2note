package speakeredit

import (
	"testing"

	"audio2note/internal/transcriber"
)

func threeSpeakers() []transcriber.Speaker {
	return []transcriber.Speaker{
		{Name: "Speaker 1", Quotes: []transcriber.Quote{{Text: "hi"}, {Text: "again"}}},
		{Name: "Speaker 2", Quotes: []transcriber.Quote{{Text: "yo"}}},
		{Name: "Speaker 3", Quotes: []transcriber.Quote{{Text: "hey"}, {Text: "there"}, {Text: "friend"}}},
	}
}

// --- initial state ---

func TestNewModel_InitialState(t *testing.T) {
	m := NewModel(threeSpeakers())
	if m.SpeakerIndex() != 0 || m.QuoteIndex() != 0 {
		t.Errorf("indices = %d,%d; want 0,0", m.SpeakerIndex(), m.QuoteIndex())
	}
	if m.Mode() != BrowseMode {
		t.Errorf("mode = %d, want BrowseMode", m.Mode())
	}
	if m.PendingRenameCount() != 0 {
		t.Errorf("PendingRenameCount = %d, want 0", m.PendingRenameCount())
	}
	if m.Saved() || m.ShouldExit() {
		t.Errorf("saved=%v shouldExit=%v; want both false", m.Saved(), m.ShouldExit())
	}
}

// --- speaker navigation ---

func TestNextSpeaker_ClampsAtEnd(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.NextSpeaker()
	if m.SpeakerIndex() != 1 {
		t.Fatalf("after one NextSpeaker: SpeakerIndex=%d", m.SpeakerIndex())
	}
	m.NextSpeaker()
	m.NextSpeaker() // already at last; should clamp
	if m.SpeakerIndex() != 2 {
		t.Errorf("after three NextSpeaker: SpeakerIndex=%d, want 2", m.SpeakerIndex())
	}
}

func TestPrevSpeaker_ClampsAtStart(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.NextSpeaker()
	m.PrevSpeaker()
	if m.SpeakerIndex() != 0 {
		t.Fatalf("after Next+Prev: SpeakerIndex=%d", m.SpeakerIndex())
	}
	m.PrevSpeaker() // clamp
	if m.SpeakerIndex() != 0 {
		t.Errorf("PrevSpeaker at 0: SpeakerIndex=%d, want 0", m.SpeakerIndex())
	}
}

func TestChangeSpeakerResetsQuoteIdx(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.NextQuote() // Speaker 1, quote 1
	if m.QuoteIndex() != 1 {
		t.Fatalf("QuoteIndex=%d, want 1", m.QuoteIndex())
	}
	m.NextSpeaker() // moves to Speaker 2 — quote idx must reset
	if m.SpeakerIndex() != 1 || m.QuoteIndex() != 0 {
		t.Errorf("after NextSpeaker: indices=%d,%d, want 1,0", m.SpeakerIndex(), m.QuoteIndex())
	}
	m.NextSpeaker()
	m.NextQuote()
	m.NextQuote() // Speaker 3, quote 2
	if m.QuoteIndex() != 2 {
		t.Fatalf("Speaker 3 QuoteIndex=%d, want 2", m.QuoteIndex())
	}
	m.PrevSpeaker() // back to Speaker 2; quote idx must reset
	if m.QuoteIndex() != 0 {
		t.Errorf("after PrevSpeaker: QuoteIndex=%d, want 0", m.QuoteIndex())
	}
}

// --- quote navigation ---

func TestNextQuote_ClampsAtEnd(t *testing.T) {
	m := NewModel(threeSpeakers()) // Speaker 1 has 2 quotes
	m.NextQuote()
	if m.QuoteIndex() != 1 {
		t.Fatalf("after NextQuote: QuoteIndex=%d", m.QuoteIndex())
	}
	m.NextQuote() // clamp
	if m.QuoteIndex() != 1 {
		t.Errorf("NextQuote at end: QuoteIndex=%d, want 1", m.QuoteIndex())
	}
}

func TestPrevQuote_ClampsAtStart(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.PrevQuote() // already at 0
	if m.QuoteIndex() != 0 {
		t.Errorf("PrevQuote at 0: QuoteIndex=%d, want 0", m.QuoteIndex())
	}
	m.NextQuote()
	m.PrevQuote()
	if m.QuoteIndex() != 0 {
		t.Errorf("after Next+Prev: QuoteIndex=%d, want 0", m.QuoteIndex())
	}
}

// --- edit flow ---

func TestBeginEdit_EntersEditMode(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.BeginEdit()
	if m.Mode() != EditMode {
		t.Errorf("Mode=%d, want EditMode", m.Mode())
	}
}

func TestCommitEdit_RecordsRename(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.BeginEdit()
	m.CommitEdit("Alice")
	if m.Mode() != BrowseMode {
		t.Errorf("Mode=%d, want BrowseMode", m.Mode())
	}
	pending, ok := m.PendingRename("Speaker 1")
	if !ok || pending != "Alice" {
		t.Errorf("PendingRename = (%q, %v), want (Alice, true)", pending, ok)
	}
}

func TestCancelEdit_NoRename(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.BeginEdit()
	m.CancelEdit()
	if m.Mode() != BrowseMode {
		t.Errorf("Mode=%d, want BrowseMode", m.Mode())
	}
	if _, ok := m.PendingRename("Speaker 1"); ok {
		t.Errorf("PendingRename should be absent after CancelEdit")
	}
}

func TestCommitEdit_EmptyStringRemovesPendingRename(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.BeginEdit()
	m.CommitEdit("Alice")
	m.BeginEdit()
	m.CommitEdit("")
	if _, ok := m.PendingRename("Speaker 1"); ok {
		t.Errorf("empty rename should remove pending: renames=%v", m.Renames())
	}
}

func TestCommitEdit_WhitespaceOnlyRemovesPendingRename(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.BeginEdit()
	m.CommitEdit("Alice")
	m.BeginEdit()
	m.CommitEdit("   ")
	if _, ok := m.PendingRename("Speaker 1"); ok {
		t.Errorf("whitespace-only rename should remove pending: renames=%v", m.Renames())
	}
}

func TestCommitEdit_SameAsOriginalRemovesPendingRename(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.BeginEdit()
	m.CommitEdit("Alice")
	m.BeginEdit()
	m.CommitEdit("Speaker 1") // back to original
	if _, ok := m.PendingRename("Speaker 1"); ok {
		t.Errorf("re-setting to original should remove pending: renames=%v", m.Renames())
	}
}

// --- save / quit ---

func TestSave_SetsSavedAndShouldExit(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.Save()
	if !m.Saved() {
		t.Error("Saved() should be true after Save()")
	}
	if !m.ShouldExit() {
		t.Error("ShouldExit() should be true after Save()")
	}
}

func TestRequestQuit_CleanReturnsQuitNow(t *testing.T) {
	m := NewModel(threeSpeakers())
	outcome := m.RequestQuit()
	if outcome != QuitNow {
		t.Errorf("outcome=%d, want QuitNow", outcome)
	}
	if !m.ShouldExit() {
		t.Error("ShouldExit() should be true after a clean RequestQuit")
	}
	if m.Saved() {
		t.Error("Saved() should remain false on a bare quit")
	}
}

func TestRequestQuit_DirtyEntersConfirm(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.BeginEdit()
	m.CommitEdit("Alice")
	outcome := m.RequestQuit()
	if outcome != QuitNeedsConfirm {
		t.Errorf("outcome=%d, want QuitNeedsConfirm", outcome)
	}
	if m.Mode() != ConfirmQuitMode {
		t.Errorf("Mode=%d, want ConfirmQuitMode", m.Mode())
	}
	if m.ShouldExit() {
		t.Error("ShouldExit() should be false while awaiting confirmation")
	}
}

func TestConfirmQuit_YesExitsUnsaved(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.BeginEdit()
	m.CommitEdit("Alice")
	m.RequestQuit()
	m.ConfirmQuit(true)
	if !m.ShouldExit() {
		t.Error("ShouldExit() should be true after ConfirmQuit(true)")
	}
	if m.Saved() {
		t.Error("Saved() should remain false after a confirmed quit")
	}
}

func TestConfirmQuit_NoReturnsToBrowse(t *testing.T) {
	m := NewModel(threeSpeakers())
	m.BeginEdit()
	m.CommitEdit("Alice")
	m.RequestQuit()
	m.ConfirmQuit(false)
	if m.Mode() != BrowseMode {
		t.Errorf("Mode=%d, want BrowseMode after ConfirmQuit(false)", m.Mode())
	}
	if m.ShouldExit() {
		t.Error("ShouldExit() should remain false after ConfirmQuit(false)")
	}
	if _, ok := m.PendingRename("Speaker 1"); !ok {
		t.Error("PendingRename should survive a cancelled quit")
	}
}

// --- queries ---

func TestCurrentSpeakerAndQuote(t *testing.T) {
	m := NewModel(threeSpeakers())
	if got := m.CurrentSpeaker().Name; got != "Speaker 1" {
		t.Errorf("CurrentSpeaker.Name = %q, want Speaker 1", got)
	}
	q, ok := m.CurrentQuote()
	if !ok || q.Text != "hi" {
		t.Errorf("CurrentQuote = (%q, %v), want (hi, true)", q.Text, ok)
	}
	m.NextSpeaker()
	if got := m.CurrentSpeaker().Name; got != "Speaker 2" {
		t.Errorf("after Next: CurrentSpeaker.Name = %q", got)
	}
}

func TestPendingRenameCount(t *testing.T) {
	m := NewModel(threeSpeakers())
	if m.PendingRenameCount() != 0 {
		t.Fatalf("initial PendingRenameCount = %d", m.PendingRenameCount())
	}
	m.BeginEdit()
	m.CommitEdit("Alice")
	m.NextSpeaker()
	m.BeginEdit()
	m.CommitEdit("Bob")
	if got := m.PendingRenameCount(); got != 2 {
		t.Errorf("PendingRenameCount = %d, want 2", got)
	}
}
