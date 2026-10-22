// Package summarizer defines the Summarizer interface and a global registry
// adapters register into via init().
package summarizer

import (
	"audio2note/internal/config"
	"audio2note/internal/registry"
	"context"
	"fmt"
	"strings"
	"time"
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
	// BasePrompt is the base instruction that opens the prompt, replacing the
	// built-in default. May contain config.TranscriptPlaceholder, which is
	// substituted with TranscriptFile. Empty means use config.DefaultBasePrompt.
	BasePrompt string
	// Highlights are timestamps (offsets from the start of the recording)
	// marked by the user as important. Empty/nil means no highlights to call
	// out.
	Highlights []time.Duration
	// RequestTitle asks the adapter to also produce a short title for the
	// meeting and return it in Result.Title. Title capture is best-effort:
	// adapters return an empty Result.Title (with nil error) when capture
	// fails, so a busted title never aborts a successful summarization.
	RequestTitle bool
}

// Result reports values the summarizer produced alongside the notes file.
type Result struct {
	// Title is the short meeting title the adapter produced when
	// Input.RequestTitle was true. Empty otherwise, or when title capture
	// failed (best-effort — see Input.RequestTitle).
	Title string
}

type Summarizer interface {
	Name() string
	Summarize(ctx context.Context, in Input) (Result, error)
}

// BuildBasePrompt assembles the shared summarizer prompt — the same text both
// the Claude and OpenCode adapters used to inline. Adapters call this and then
// layer on whatever delivery-mechanism instructions they need for title
// capture (Claude relies on its --json-schema flag, opencode appends a
// "write the title to <path>" instruction).
//
// The prompt opens with the base instruction (in.BasePrompt, or
// config.DefaultBasePrompt when unset). config.TranscriptPlaceholder in the base
// instruction is replaced with the transcript path; if the placeholder is
// absent, the path is appended so the model is never left without it. The
// conditional fragments below the base (output file, vault, highlights, title,
// appended prompt) are always added programmatically.
func BuildBasePrompt(in Input) string {
	base := in.BasePrompt
	if base == "" {
		base = config.DefaultBasePrompt
	}

	var b strings.Builder
	if strings.Contains(base, config.TranscriptPlaceholder) {
		b.WriteString(strings.ReplaceAll(base, config.TranscriptPlaceholder, in.TranscriptFile))
	} else {
		b.WriteString(base)
		fmt.Fprintf(&b, " The transcript is located at %s.", in.TranscriptFile)
	}

	if in.OutputFile != "" {
		fmt.Fprintf(&b, " The output file for this transcript should be named %s.", in.OutputFile)
	}
	if in.ObsidianVault != "" {
		fmt.Fprintf(&b, "\n\nWrite the meeting summary to my Obsidian vault located at %s.", in.ObsidianVault)
	}
	if len(in.Highlights) > 0 {
		b.WriteString("\n\nThe user marked the following moments in this recording as highlights:")
		for _, h := range in.Highlights {
			fmt.Fprintf(&b, "\n  - %s", FormatHighlight(h))
		}
		b.WriteString("\n\nPay special attention to the transcript content around these timestamps; treat them as the most important moments in the discussion.")
	}
	if in.RequestTitle {
		b.WriteString("\n\nAlso produce a short, descriptive title for this meeting (5–10 words, no quotes, no trailing punctuation, suitable as a filename).")
	}
	if in.ExtraPrompt != "" {
		fmt.Fprintf(&b, "\n\n%s", in.ExtraPrompt)
	}
	return b.String()
}

// FormatHighlight renders a duration as HH:MM:SS (zero-padded).
func FormatHighlight(d time.Duration) string {
	total := int(d.Round(time.Second).Seconds())
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

var reg = registry.New[Summarizer]()

func Register(s Summarizer)               { reg.Register(s) }
func Get(name string) (Summarizer, error) { return reg.Get(name) }
func Names() []string                     { return reg.Names() }
