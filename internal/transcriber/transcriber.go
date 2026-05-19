// Package transcriber defines the Transcriber interface and a global registry
// adapters register into via init().
package transcriber

import (
	"context"

	"audio2note/internal/registry"
)

type Transcriber interface {
	Name() string
	DefaultTranscriptFile() string
	// Transcribe reads audioFile and writes the transcript to outputFile
	// (an absolute path chosen by the caller).
	Transcribe(ctx context.Context, audioFile, outputFile string) error
}

var reg = registry.New[Transcriber]()

func Register(t Transcriber)              { reg.Register(t) }
func Get(name string) (Transcriber, error) { return reg.Get(name) }
func Names() []string                      { return reg.Names() }
