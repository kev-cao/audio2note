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
| `--transcribe-only` | off | Run only the transcriber and exit. Mutually exclusive with `--summarize-only`. |
| `--summarize-only` | off | Run only the summarizer against an existing transcript file (audio file becomes optional). Mutually exclusive with `--transcribe-only`. |
| `--transcriber-args <string>` | `""` | Extra args to forward to the transcriber CLI. Shell-quoted; split with `shlex` and appended after the adapter's defaults. |
| `--summarizer-args <string>` | `""` | Same, for the summarizer CLI. |
| `--transcriber-no-defaults` | off | Skip the transcriber adapter's hardcoded extras (e.g. `--persist` for `macwhisper`). Core args (the audio file) are still passed. |
| `--summarizer-no-defaults` | off | Skip the summarizer adapter's hardcoded extras (e.g. `--permission-mode acceptEdits` and `--add-dir <vault>` for `claudecode`). The prompt is still passed. |
| `--prompt <string>` | `""` | Extra text appended to the summarizer's prompt template. Lets you shape the output style without editing adapter source. |
| `--edit-speaker-labels` | off | Between transcribe and summarize, open an interactive TUI to rename speaker labels in the transcript. Requires a transcriber that supports speaker labels (currently `macwhisper` with a `.json` transcript). |

The hardcoded extras each adapter sets are listed in the adapter source — see `internal/adapters/macwhisper/macwhisper.go` and `internal/adapters/claudecode/claudecode.go`. Forwarding the same flag with a different value is the normal override path; both `mw` and `claude` honor last-wins for repeated flags.

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

# Only transcribe (skip summarization)
audio2note --transcribe-only meeting.mp3

# Only summarize an existing transcript (no audio file needed)
audio2note --summarize-only --transcript-file /tmp/meeting-transcript.txt

# Forward flags to the underlying tools
audio2note \
  --transcriber-args "--model whisper-large" \
  --summarizer-args "--model claude-sonnet-4-6" \
  meeting.mp3

# Drop the adapter's hardcoded extras and run claude with your own posture
audio2note \
  --summarizer-no-defaults \
  --summarizer-args "--dangerously-skip-permissions" \
  meeting.mp3

# Steer the summary style with extra prompt text
audio2note \
  --prompt "Use bullet points grouped by speaker. Keep the whole summary under 200 words." \
  meeting.mp3

# Rename "Speaker 1"/"Speaker 2"/etc. before summarization (TUI)
audio2note \
  --summarize-only \
  --transcript-file meeting.json \
  --edit-speaker-labels \
  --obsidian
```

### Speaker-label editing TUI

When `--edit-speaker-labels` is set, the pipeline opens an interactive prompt for each speaker in the transcript. You can page through quotes from that speaker to figure out who they are, rename them, and save. Keys:

- `↑` / `↓` (or `k` / `j`) — previous / next speaker
- `←` / `→` (or `h` / `l`) — previous / next quote within the current speaker
- `Enter` — start editing the current speaker's name
  - In edit mode: `Enter` confirms, `Esc` cancels
- `Ctrl+S` — save renames and continue the pipeline
- `q` (or `Ctrl+C`) — quit; prompts for confirmation if you have unsaved renames

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
type Input struct {
    AudioFile  string   // absolute path
    OutputFile string   // absolute path
    ExtraArgs  []string // already-split tokens to append to the tool's argv
    NoDefaults bool     // skip the adapter's hardcoded extra flags
}

type Transcriber interface {
    Name() string
    DefaultTranscriptFile() string
    Transcribe(ctx context.Context, in Input) error
}

// internal/summarizer/summarizer.go
type Input struct {
    TranscriptFile string   // absolute path
    OutputFile     string   // file name (not a path)
    ObsidianVault  string   // empty when not writing to a vault
    ExtraArgs      []string // already-split tokens to append to the tool's argv
    NoDefaults     bool     // skip the adapter's hardcoded extra flags
    ExtraPrompt    string   // user-supplied text to append to the prompt
}

type Summarizer interface {
    Name() string
    Summarize(ctx context.Context, in Input) error
}
```

Notes:

- The transcriber is given an absolute `OutputFile`; how it actually lands the transcript there (stdout redirect, a CLI `-o` flag, etc.) is the adapter's call. The `macwhisper` adapter redirects mw's stdout to the file because `mw transcribe` has no output flag.
- Adapters should honor `ExtraArgs` (append after the adapter's defaults) and `NoDefaults` (skip the hardcoded extras when set). This is what makes `--transcriber-args` / `--summarizer-args` and the matching `--*-no-defaults` toggles work uniformly across adapters.
- Both Input structs leave room for future fields (model selection, system prompt overrides, etc.) to be added without breaking adapters.

### Optional capability: speaker-label editing

Transcribers that can parse and rewrite their output for speaker editing implement `transcriber.SpeakerLabelEditor`:

```go
type SpeakerLabelEditor interface {
    HasSpeakerLabels(path string) (bool, error)
    LoadSpeakers(path string) ([]Speaker, error)
    ApplyRenames(path string, renames map[string]string) error
}
```

`--edit-speaker-labels` type-asserts the active transcriber into this interface; transcribers that don't implement it get a clear error. The `macwhisper` adapter implements it for JSON transcripts; rewrites preserve all per-segment fields verbatim by round-tripping through `json.RawMessage`.
