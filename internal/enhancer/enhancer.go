// Package enhancer defines the Enhancer interface and a global registry
// adapters register into via init(). An enhancer preprocesses an audio file
// (e.g. isolating speech from background noise) before transcription.
package enhancer

import (
	"audio2note/internal/registry"
	"context"
)

// Input is everything an enhancer needs to produce a cleaned-up audio file.
type Input struct {
	// AudioFile is the absolute path to the source audio.
	AudioFile string
	// OutputDir is a directory the enhancer may write its output (and any
	// intermediate files) into. The enhancer owns the layout beneath it and
	// returns the absolute path of the enhanced file.
	OutputDir string
}

// Enhancer cleans up an audio file to improve downstream transcription and
// returns the absolute path to the enhanced audio.
type Enhancer interface {
	Name() string
	// Validate checks the adapter's external dependencies (e.g. required CLI
	// binaries) are available. Callers invoke it up front so a run can
	// fast-fail before doing expensive work like downloading or stitching
	// audio, rather than discovering the missing tool mid-pipeline.
	Validate() error
	Enhance(ctx context.Context, in Input) (string, error)
}

var reg = registry.New[Enhancer]()

func Register(e Enhancer)               { reg.Register(e) }
func Get(name string) (Enhancer, error) { return reg.Get(name) }
func Names() []string                   { return reg.Names() }
