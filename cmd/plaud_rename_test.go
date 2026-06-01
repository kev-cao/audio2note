package cmd

import (
	"audio2note/internal/plaud"
	"context"
	"errors"
	"testing"
)

func defaultNamed(id plaud.ID, name string) plaud.File {
	return plaud.File{ID: id, Filename: name, Extension: ".mp3"}
}

func TestRenamePlaudFiles_EmptyTitle_NoCalls(t *testing.T) {
	fp := &richFakePlaud{}
	renamePlaudFiles(context.Background(), fp, []plaud.File{defaultNamed("a", "2025-03-18 14:30:22")}, "")
	if len(fp.renameCalls) != 0 {
		t.Errorf("rename should not run with empty title; calls = %d", len(fp.renameCalls))
	}
}

func TestRenamePlaudFiles_SingleFile_DefaultName(t *testing.T) {
	fp := &richFakePlaud{}
	renamePlaudFiles(
		context.Background(), fp,
		[]plaud.File{defaultNamed("abc", "2025-03-18 14:30:22")},
		"Quarterly review",
	)
	if len(fp.renameCalls) != 1 {
		t.Fatalf("rename calls = %d, want 1", len(fp.renameCalls))
	}
	if fp.renameCalls[0].id != "abc" || fp.renameCalls[0].newName != "Quarterly review" {
		t.Errorf("rename call = %+v", fp.renameCalls[0])
	}
}

func TestRenamePlaudFiles_SkipsNonDefaultName(t *testing.T) {
	fp := &richFakePlaud{}
	renamePlaudFiles(
		context.Background(), fp,
		[]plaud.File{defaultNamed("abc", "Already named")},
		"Quarterly review",
	)
	if len(fp.renameCalls) != 0 {
		t.Errorf("rename should be skipped when filename isn't default; got %d calls", len(fp.renameCalls))
	}
}

func TestRenamePlaudFiles_MultiFile_PartSuffixes(t *testing.T) {
	fp := &richFakePlaud{}
	renamePlaudFiles(
		context.Background(), fp,
		[]plaud.File{
			defaultNamed("a", "2025-03-18 14:30:22"),
			defaultNamed("b", "2025-03-18 15:00:00"),
			defaultNamed("c", "2025-03-18 15:30:00"),
		},
		"Quarterly review",
	)
	if len(fp.renameCalls) != 3 {
		t.Fatalf("rename calls = %d, want 3", len(fp.renameCalls))
	}
	wants := []string{
		"Quarterly review (part 1/3)",
		"Quarterly review (part 2/3)",
		"Quarterly review (part 3/3)",
	}
	for i, c := range fp.renameCalls {
		if c.newName != wants[i] {
			t.Errorf("call[%d].newName = %q, want %q", i, c.newName, wants[i])
		}
	}
}

func TestRenamePlaudFiles_MultiFile_MixedDefaultAndManual(t *testing.T) {
	fp := &richFakePlaud{}
	renamePlaudFiles(
		context.Background(), fp,
		[]plaud.File{
			defaultNamed("a", "2025-03-18 14:30:22"),
			defaultNamed("b", "User renamed this"),
			defaultNamed("c", "2025-03-18 15:30:00"),
		},
		"Quarterly review",
	)
	if len(fp.renameCalls) != 2 {
		t.Fatalf("rename calls = %d, want 2 (middle one skipped)", len(fp.renameCalls))
	}
	// The skipped file shouldn't shift the suffix numbering — the index in
	// the selection list is what `(part N/M)` reports, so the renamed files
	// keep their original 1/3 and 3/3 markers. This matches the user's
	// mental model of "three recordings, two got renamed."
	if fp.renameCalls[0].newName != "Quarterly review (part 1/3)" {
		t.Errorf("first call newName = %q, want suffix (part 1/3)", fp.renameCalls[0].newName)
	}
	if fp.renameCalls[1].newName != "Quarterly review (part 3/3)" {
		t.Errorf("second call newName = %q, want suffix (part 3/3)", fp.renameCalls[1].newName)
	}
}

func TestRenamePlaudFiles_APIErrorKeepsGoing(t *testing.T) {
	fp := &richFakePlaud{renameErr: errors.New("plaud down")}
	renamePlaudFiles(
		context.Background(), fp,
		[]plaud.File{
			defaultNamed("a", "2025-03-18 14:30:22"),
			defaultNamed("b", "2025-03-18 15:00:00"),
		},
		"Quarterly review",
	)
	// Both rename calls should still fire even though the first errored —
	// best-effort means we don't short-circuit the loop.
	if len(fp.renameCalls) != 2 {
		t.Errorf("rename calls = %d, want 2 even with errors", len(fp.renameCalls))
	}
}
