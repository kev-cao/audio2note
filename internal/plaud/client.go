package plaud

import (
	"context"
	"time"
)

// ID is a Plaud recording identifier — a hex string assigned by the service.
type ID string

// File is a recording listed in a user's Plaud library.
type File struct {
	ID            ID
	Filename      string
	Extension     string // ".mp3", ".ogg", etc. — derived from the storage name
	Duration      time.Duration
	HasTranscript bool
}

// Highlight is a user-marked moment in a recording, expressed as an offset
// from the start.
type Highlight time.Duration

// Client is the abstraction over the Plaud API used elsewhere in audio2note.
// Implementations may hit the real service or be mocked for tests.
type Client interface {
	// Validate confirms the configured bearer token is still accepted by
	// Plaud. Returns nil on success.
	Validate(ctx context.Context) error

	// ListFiles returns the recordings in the authenticated user's library.
	ListFiles(ctx context.Context) ([]File, error)

	// DownloadRecording downloads the audio for the given recording to
	// outputFilePath and returns the absolute path on disk.
	DownloadRecording(ctx context.Context, id ID, outputFilePath string) (string, error)

	// DownloadTranscript downloads the speaker-attributed transcript for the
	// recording to outputFilePath and returns the absolute path on disk. The
	// file is written as plain JSON (Plaud stores it gzipped; the
	// decompression happens during download).
	DownloadTranscript(ctx context.Context, id ID, outputFilePath string) (string, error)

	// GetHighlights returns the user-marked highlight timestamps for the
	// recording.
	GetHighlights(ctx context.Context, id ID) ([]Highlight, error)

	// RenameFile updates the filename ("title") of the recording on Plaud.
	// Used by `audio2note plaud run` after summarization to replace the
	// default `YYYY-MM-DD HH:MM:SS` timestamp with a meaningful title.
	RenameFile(ctx context.Context, id ID, newName string) error
}
