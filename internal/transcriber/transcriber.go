// Package transcriber defines the Transcriber interface and a global registry
// adapters register into via init().
package transcriber

import (
	"context"

	"audio2note/internal/registry"
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
	DefaultTranscriptFile() string
	Transcribe(ctx context.Context, in Input) error
}

var reg = registry.New[Transcriber]()

func Register(t Transcriber)              { reg.Register(t) }
func Get(name string) (Transcriber, error) { return reg.Get(name) }
func Names() []string                      { return reg.Names() }
