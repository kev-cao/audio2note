// Package transcriber defines the Transcriber interface and a global registry
// adapters register into via init().
package transcriber

import (
	"audio2note/internal/registry"
	"context"
)

// Input is everything a transcriber needs to produce a transcript.
type Input struct {
	// AudioFile is the absolute path to the input audio.
	AudioFile string
	// OutputFile is the absolute path where the transcript should be written.
	OutputFile string
	// ExtraArgs are already-split tokens to append to the underlying tool's
	// command line.
	ExtraArgs []string
	// NoDefaults tells the adapter to skip its hardcoded extra flags (the
	// core args — audio file and output destination — are still passed).
	NoDefaults bool
}

type Transcriber interface {
	Name() string
	Transcribe(ctx context.Context, in Input) error
}

// Quote is a single utterance attributed to a speaker.
type Quote struct {
	Text string
}

// Speaker is a labeled speaker plus everything they said, in transcript order.
type Speaker struct {
	Name   string
	Quotes []Quote
}

// SpeakerLabelEditor is an optional capability implemented by transcribers
// whose output file can be parsed for editable speaker labels and rewritten
// in place. The pipeline type-asserts the active transcriber into this
// interface only when --edit-speaker-labels is set.
type SpeakerLabelEditor interface {
	// HasSpeakerLabels validates that path is in a format this adapter
	// recognizes as containing speaker labels. Returns false (not an error)
	// when the format is valid for the transcriber but lacks labels.
	HasSpeakerLabels(path string) (bool, error)
	// LoadSpeakers parses the file and returns speakers in order of first
	// appearance; each speaker's Quotes are in transcript order.
	LoadSpeakers(path string) ([]Speaker, error)
	// ApplyRenames rewrites the file atomically with the given map
	// (old → new). Unmentioned speakers are left untouched. Empty map is a
	// no-op.
	ApplyRenames(path string, renames map[string]string) error
}

var reg = registry.New[Transcriber]()

func Register(t Transcriber)               { reg.Register(t) }
func Get(name string) (Transcriber, error) { return reg.Get(name) }
func Names() []string                      { return reg.Names() }
