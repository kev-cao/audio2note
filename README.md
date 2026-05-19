# audio2note

A small Go CLI that turns an audio file into meeting notes by chaining a transcriber and an AI summarizer.

```
audio file ──► transcriber ──► transcript.txt ──► summarizer ──► notes.md
```

v1 ships with two adapters:

- **macwhisper** — shells out to the MacWhisper CLI (`mw transcribe`)
- **claudecode** — shells out to Claude Code (`claude -p ...`) to write the notes

The project is built around `Transcriber` and `Summarizer` interfaces so swapping either side is a one-file change.

## Prerequisites

- Go 1.22+
- [MacWhisper](https://goodsnooze.gumroad.com/l/macwhisper) with the `mw` CLI on `PATH`
- [Claude Code](https://docs.claude.com/en/docs/claude-code) with the `claude` CLI on `PATH`

## Build

```sh
go build -o audio2note .
```

## Usage

```sh
audio2note meeting.mp3
```

That runs the defaults: `mw transcribe meeting.mp3` writes the transcript to `Transcript.txt` in the current directory, then `claude` reads it and writes `transcript-notes.md`.

### Flags

| Flag | Default | Behavior |
| --- | --- | --- |
| `--transcriber` | `macwhisper` | Transcriber adapter to use. |
| `--summarizer` | `claudecode` | Summarizer adapter to use. |
| `--transcript-file <path>` | transcriber's default (`Transcript.txt` for `macwhisper`) | Where the transcript is written and where the summarizer reads from. |
| `--obsidian` | off | Tell the summarizer to write the notes into the Obsidian vault at `$OBSIDIAN_VAULT_PATH`. Errors if the env var is empty. |
| `--obsidian-vault <path>` | unset | Explicit vault path. Overrides `$OBSIDIAN_VAULT_PATH` and implies `--obsidian`. |
| `--output-file <name>` | `transcript-notes.md` | File name passed to the summarizer for the notes output. |

### Examples

```sh
# Default end-to-end run
audio2note meeting.mp3

# Write notes into the Obsidian vault from $OBSIDIAN_VAULT_PATH
audio2note --obsidian meeting.mp3

# Write notes into a specific vault (implies --obsidian)
audio2note --obsidian-vault ~/vaults/work meeting.mp3

# Use an explicit transcript path and output filename
audio2note \
  --transcript-file /tmp/meeting-transcript.txt \
  --output-file 2026-05-19-standup.md \
  meeting.mp3
```

## Adding a new transcriber or summarizer

The whole point of the layout is that adding an adapter is small and local:

1. Create `internal/adapters/<name>/<name>.go`.
2. Implement either `transcriber.Transcriber` or `summarizer.Summarizer`.
3. Register the adapter in an `init()`:
   ```go
   func init() { transcriber.Register(MyAdapter{}) }
   ```
4. Add one blank-import line to `cmd/root.go`:
   ```go
   _ "audio2note/internal/adapters/<name>"
   ```

That's it — the new adapter is now selectable via `--transcriber <name>` or `--summarizer <name>`, and `--help`-style errors will list it as an available option.

### Interfaces

```go
// internal/transcriber/transcriber.go
type Transcriber interface {
    Name() string
    DefaultTranscriptFile() string
    Transcribe(ctx context.Context, audioFile, outputFile string) error
}

// internal/summarizer/summarizer.go
type Input struct {
    TranscriptFile string // absolute path
    OutputFile     string // file name (not a path)
    ObsidianVault  string // empty when not writing to a vault
}

type Summarizer interface {
    Name() string
    Summarize(ctx context.Context, in Input) error
}
```

Notes:

- The transcriber is given an absolute `outputFile`; how it actually lands the transcript there (stdout redirect, a CLI `-o` flag, etc.) is the adapter's call. The `macwhisper` adapter redirects mw's stdout to the file because `mw transcribe` has no output flag.
- The summarizer receives a struct so future fields (model selection, system prompt overrides, etc.) can be added without breaking adapters.
