// Package summarizer defines the Summarizer interface and a global registry
// adapters register into via init().
package summarizer

import (
	"context"
	"time"

	"audio2note/internal/registry"
)

// Input is everything a summarizer needs to produce meeting notes.
type Input struct {
	// TranscriptFile is the absolute path to the transcript to read.
	TranscriptFile string
	// OutputFile is the file name (not a path) that the summarizer should
	// write the notes to.
	OutputFile string
	// ObsidianVault is the absolute path to an Obsidian vault to write the
	// notes into. Empty when the caller does not want vault output.
	ObsidianVault string
	// ExtraArgs are already-split tokens to append to the underlying tool's
	// command line.
	ExtraArgs []string
	// NoDefaults tells the adapter to skip its hardcoded extra flags (the
	// core args — the prompt — are still passed).
	NoDefaults bool
	// ExtraPrompt is user-supplied text appended to the summarizer's prompt
	// template. Empty string means no addition.
	ExtraPrompt string
	// Highlights are timestamps (offsets from the start of the recording)
	// marked by the user as important. Empty/nil means no highlights to call
	// out.
	Highlights []time.Duration
}

type Summarizer interface {
	Name() string
	Summarize(ctx context.Context, in Input) error
}

var reg = registry.New[Summarizer]()

func Register(s Summarizer)               { reg.Register(s) }
func Get(name string) (Summarizer, error) { return reg.Get(name) }
func Names() []string                     { return reg.Names() }
