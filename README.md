# audio2note

A small Go CLI that turns an audio file into meeting notes by chaining a transcriber and an AI summarizer.

```
audio file ──► transcriber ──► transcript.json ──► summarizer ──► notes.md
```

The audio can come from two sources:

- **local files** — `audio2note run <file>...`
- **Plaud recordings** — `audio2note plaud run` pulls recordings straight from your [Plaud](https://www.plaud.ai/) library (no manual export needed)

Pluggable adapters sit on each side of the pipeline:

| Role | Adapters |
| --- | --- |
| Transcriber | `macwhisper` (shells out to the MacWhisper `mw` CLI), `whisperx` (shells out to `whisperx`) |
| Summarizer | `claudecode` (shells out to `claude -p ...`), `opencode` (shells out to `opencode run ...`) |
| Audio preprocessing (optional) | `resemble-enhance` (denoise / enhance before transcription) |

The project is built around `Transcriber` and `Summarizer` interfaces so swapping either side is a one-file change — see [Adding a new transcriber or summarizer](#adding-a-new-transcriber-or-summarizer).

## Prerequisites

You need Go to build, one transcriber, and one summarizer. The other entries are
only needed for specific features.

- **Go 1.22+** — to build the binary.
- **A transcriber** (at least one):
  - [MacWhisper](https://goodsnooze.gumroad.com/l/macwhisper) with the `mw` CLI on `PATH` — **the default**, or
  - [WhisperX](https://github.com/m-bain/whisperX) with the `whisperx` CLI on `PATH`
- **A summarizer** (at least one):
  - [Claude Code](https://docs.claude.com/en/docs/claude-code) with the `claude` CLI on `PATH` — **the default**, or
  - [OpenCode](https://opencode.ai/) with the `opencode` CLI on `PATH`
- [ffmpeg](https://ffmpeg.org/) on `PATH` — only when stitching multiple audio files in one invocation (`brew install ffmpeg`).
- _Optional:_ the [resemble-enhance](https://github.com/resemble-ai/resemble-enhance) CLI on `PATH` (and `git-lfs` for its one-time model download) — only for `--denoise` / `--enhance-audio`.
- _Optional:_ a [Plaud](https://www.plaud.ai/) account — only for the `plaud` commands (see [Plaud setup](#plaud-setup)).

The out-of-the-box default (`audio2note run <file>`) uses **macwhisper + claudecode**, so the quickest start is to install the `mw` and `claude` CLIs. You only need the tools for the adapters you actually use — swap either side per run with `--transcriber` / `--summarizer`, or set a persistent default in the [config file](#configuration).

### whisperx notes

The `whisperx` adapter defaults to `--model small --device cuda --output_format json --diarize`, so out of the box it expects a CUDA GPU and speaker diarization. Two things to know:

- **Diarization needs HuggingFace auth.** `--diarize` requires either `$HF_TOKEN` set or a cached login (`hf auth login`). The adapter pre-checks this and fails with a clear message instead of letting `whisperx` error downstream.
- **No GPU?** Override the device with `--transcriber-args "--device cpu"` (repeated flags are last-wins), or pass `--transcriber-no-defaults` and supply your own flags.

## Build

```sh
go build -o bin/audio2note .
```

## Usage: local files

```sh
audio2note run meeting.mp3
```

That runs the defaults: `mw transcribe meeting.mp3` writes the transcript to a temp file, then `claude` reads it and writes the meeting notes (claude picks the filename unless you pass `--output-file`).

### Multiple audio files

Pass several files and they're stitched in the given order with `ffmpeg` (stream copy, no re-encode) before being sent to the transcriber:

```sh
audio2note run part1.mp3 part2.mp3 part3.mp3
```

All inputs must share the same extension/codec.

## Usage: Plaud

`audio2note plaud` pulls recordings directly from your Plaud library. Subcommands:

| Command | What it does |
| --- | --- |
| `plaud auth setup` | Prompt for a Plaud bearer token and save it to the config file (`plaudToken`). |
| `plaud auth validate` | Check that the saved token is still accepted by Plaud. |
| `plaud run` | List your library, pick one or more recordings in a TUI, then run the transcribe + summarize pipeline. |

### Plaud setup

The Plaud commands authenticate with a bearer token scraped from the Plaud web app (there is no official API). To get your token:

1. Open [https://web.plaud.ai](https://web.plaud.ai) in your browser and sign in.
2. Open the browser dev tools and inspect **localStorage** for the site.
3. Copy the value of the key `pid_tokenstr`.

Then save it:

```sh
audio2note plaud auth setup
# paste the token when prompted; it's stored as plaudToken in ~/.audio2note/config.json (0600)
```

Confirm it works:

```sh
audio2note plaud auth validate
# → token verified!
```

The token eventually expires; when it does, `plaud run` and `plaud auth validate` tell you to re-run `plaud auth setup` with a fresh token.

### Running against a Plaud recording

```sh
audio2note plaud run
```

This fetches your library, opens an interactive picker, and then runs the same pipeline the local-file flow uses. `plaud run` accepts all of the shared [flags](#flags) plus one Plaud-specific flag, `--no-rename` (see below).

What `plaud run` does beyond the local flow:

- **Interactive picker.** The picker is multi-select: `space` toggles a recording in/out of an ordered selection, `enter` confirms. With no explicit toggles, `enter` just selects the highlighted row (single-file flow stays one keystroke). Type to filter. A `✓ transcript` marker shows which recordings already have a Plaud transcript.
- **Downloads what it needs.** Normally it downloads the recording's audio to a temp file and transcribes it. With `--summarize-only`, it downloads the existing Plaud transcript instead (or reuses a local `--transcript-file` if one already exists).
- **Stitches multi-selections.** Selecting several recordings downloads each and stitches them (via `ffmpeg`) into one timeline before transcribing. All selected recordings must share the same format.
- **Merges highlights.** Moments you marked as highlights in the Plaud app are fetched and passed to the summarizer as important timestamps (offsets are adjusted so they line up with the stitched timeline). The summarizer is told to pay special attention to the transcript around those moments.
- **Auto-renames the recording.** After a successful summarize, the recording's Plaud filename is renamed to the summarizer-produced title (replacing Plaud's default `YYYY-MM-DD HH:MM:SS` name). Multi-file selections get `(part N/M)` suffixes. This is best-effort — a recording you already renamed manually is left alone, and per-file errors surface as warnings without aborting. Disable it with `--no-rename`.

### Plaud examples

```sh
# Pick a recording and run the full pipeline (transcribe → summarize → rename)
audio2note plaud run

# Write the notes into your Obsidian vault and don't rename the Plaud recording
audio2note plaud run --obsidian --no-rename

# Only transcribe the picked recording(s) (skip summarization and rename)
audio2note plaud run --transcribe-only

# Summarize using the transcript Plaud already has (no audio download)
audio2note plaud run --summarize-only
```

## Flags

These flags are shared by both `run` and `plaud run`. (`--no-rename` is `plaud run`-only.)

| Flag | Default | Behavior |
| --- | --- | --- |
| `--transcriber` | `macwhisper` | Transcriber adapter to use (`macwhisper` or `whisperx`). |
| `--summarizer` | `claudecode` | Summarizer adapter to use (`claudecode` or `opencode`). |
| `--transcript-file <path>` | a temp file (`$TMPDIR/audio2note-transcript.json`) | Where the transcript is written and where the summarizer reads from. |
| `--obsidian` | off | Write the notes into the Obsidian vault configured as `obsidianVault` in the [config file](#configuration). Errors if `obsidianVault` is unset. |
| `--obsidian-vault <path>` | unset | Explicit vault path for this run. Overrides the config `obsidianVault` and implies `--obsidian`. |
| `--output-file <name>` | _(summarizer chooses)_ | If set, the summarizer is told to write the notes to this exact file name. Otherwise the summarizer picks one itself. |
| `--transcribe-only` | off | Run only the transcriber and exit. Mutually exclusive with `--summarize-only`. |
| `--summarize-only` | off | Run only the summarizer against an existing transcript file (audio file becomes optional). Mutually exclusive with `--transcribe-only`. |
| `--transcriber-args <string>` | `""` | Extra args to forward to the transcriber CLI. Shell-quoted; split with `shlex` and appended after the adapter's defaults. |
| `--summarizer-args <string>` | `""` | Same, for the summarizer CLI. |
| `--transcriber-no-defaults` | off | Skip the transcriber adapter's hardcoded extras (e.g. `--persist --format json` for `macwhisper`). Core args (the audio file) are still passed. |
| `--summarizer-no-defaults` | off | Skip the summarizer adapter's hardcoded extras (e.g. `--permission-mode acceptEdits` and `--add-dir <vault>` for `claudecode`). The prompt is still passed. |
| `--append-prompt <string>` | `""` | Extra text appended _after_ the base prompt, to shape the output style for one run. To change the base prompt itself, set `basePrompt` in the [config file](#configuration). |
| `--edit-speaker-labels` | off | Between transcribe and summarize, open an interactive TUI to rename speaker labels in the transcript. Requires a transcriber that supports speaker labels (currently `macwhisper` with a `.json` transcript). |
| `--denoise` | off | Before transcription, denoise speech with [resemble-enhance](https://github.com/resemble-ai/resemble-enhance) (its denoiser only — fast, non-generative). Requires the `resemble-enhance` CLI on `PATH` (and `git-lfs` for its one-time model download). Uses CUDA when available, else Apple MPS, else CPU. Mutually exclusive with `--enhance-audio` and `--summarize-only`. |
| `--enhance-audio` | off | **Experimental.** Before transcription, run the full [resemble-enhance](https://github.com/resemble-ai/resemble-enhance) pipeline (denoise **+** generative restoration). The enhancer *resynthesizes* speech from a generative prior, so it can alter words and **hurt transcription accuracy** — prefer `--denoise`. Much slower than `--denoise`. Same requirements. Runs on CUDA when available, otherwise CPU (this path isn't supported on Apple MPS). Mutually exclusive with `--denoise` and `--summarize-only`. |
| `--no-rename` _(`plaud run` only)_ | off | Disable auto-renaming the Plaud recording to the summarizer-produced title after a successful run. |

The hardcoded extras each adapter sets are listed in the adapter source — see `internal/adapters/<name>/<name>.go`. Forwarding the same flag with a different value is the normal override path; the underlying CLIs honor last-wins for repeated flags.

## Configuration

Persistent defaults live in `~/.audio2note/config.json`. The file is optional — a
missing file (or a missing key) falls back to the built-in default — so you only
store what you want to override. Precedence for every setting is: **explicit flag
→ config file → built-in default** (an explicit flag always wins).

Manage it with the `config` subcommands rather than hand-editing:

```sh
audio2note config help                       # list every key with its help
audio2note config help defaultTranscriber    # describe one key (restricted keys list their options)
audio2note config get                        # print all effective values
audio2note config get basePrompt             # print one effective value
audio2note config set defaultSummarizer opencode
```

`config set` validates restricted keys (rejects, e.g., an unknown transcriber) and
writes only the keys you've set (file stays minimal). `config get` shows the
*effective* value — your setting if present, otherwise the default.

### Keys

| Key | Default | Notes |
| --- | --- | --- |
| `defaultTranscriber` | `macwhisper` | Used when `--transcriber` isn't passed. One of `macwhisper`, `whisperx`. |
| `defaultSummarizer` | `claudecode` | The default AI harness, used when `--summarizer` isn't passed. One of `claudecode`, `opencode`. |
| `basePrompt` | _(generic, see below)_ | The base instruction the summarizer opens with. Replaces the built-in default. |
| `obsidianVault` | unset | Default Obsidian vault path. When set, notes go to the vault by default (no flag needed). `--obsidian-vault` overrides it per run; `--obsidian` forces vault output using it. |
| `plaudToken` | unset | Your Plaud bearer token, used by the `plaud` commands. Set it with [`plaud auth setup`](#plaud-setup), not `config set`; `config get` shows it masked. |

### Base prompt and `{{transcript}}`

The built-in default base prompt is intentionally generic:

```
Read through the transcript located at {{transcript}} and generate a summary.
```

`basePrompt` replaces this opening instruction. The `{{transcript}}` placeholder is
substituted with the transcript's path; if you omit the placeholder, the path is
appended automatically so the model always knows where to read. The structural
parts of the prompt the pipeline needs (output filename, Obsidian vault,
highlights, title request) are still added automatically after your base prompt,
and `--append-prompt` adds one-off text after that.

```sh
audio2note config set basePrompt "Summarize {{transcript}} as terse bullet points grouped by speaker."
```

## Examples

```sh
# Default end-to-end run (local file)
audio2note run meeting.mp3

# Write notes into the Obsidian vault configured in the config file
audio2note run --obsidian meeting.mp3

# Write notes into a specific vault (implies --obsidian)
audio2note run --obsidian-vault ~/vaults/work meeting.mp3

# Use an explicit transcript path and output filename
audio2note run \
  --transcript-file /tmp/meeting-transcript.json \
  --output-file 2026-05-19-standup.md \
  meeting.mp3

# Transcribe with whisperx instead of macwhisper, and summarize with opencode
audio2note run \
  --transcriber whisperx \
  --summarizer opencode \
  meeting.mp3

# Only transcribe (skip summarization)
audio2note run --transcribe-only meeting.mp3

# Stitch multiple recordings into one before transcribing (requires ffmpeg)
audio2note run part1.mp3 part2.mp3 part3.mp3

# Only summarize an existing transcript (no audio file needed)
audio2note run --summarize-only --transcript-file /tmp/meeting-transcript.json

# Forward flags to the underlying tools
audio2note run \
  --transcriber-args "--model whisper-large" \
  --summarizer-args "--model claude-sonnet-4-6" \
  meeting.mp3

# Drop the adapter's hardcoded extras and run claude with your own posture
audio2note run \
  --summarizer-no-defaults \
  --summarizer-args "--dangerously-skip-permissions" \
  meeting.mp3

# Steer the summary style with extra prompt text (appended after the base prompt)
audio2note run \
  --append-prompt "Use bullet points grouped by speaker. Keep the whole summary under 200 words." \
  meeting.mp3

# Pick a Plaud recording and run the full pipeline
audio2note plaud run --obsidian

# Rename "Speaker 1"/"Speaker 2"/etc. before summarization (TUI)
audio2note run \
  --summarize-only \
  --transcript-file meeting.json \
  --edit-speaker-labels \
  --obsidian
```

## Speaker-label editing TUI

When `--edit-speaker-labels` is set, the pipeline opens an interactive prompt for each speaker in the transcript. You can page through quotes from that speaker to figure out who they are, rename them, and save. Keys:

- `↑` / `↓` (or `k` / `j`) — previous / next speaker
- `←` / `→` (or `h` / `l`) — previous / next quote within the current speaker
- `Enter` — start editing the current speaker's name
  - In edit mode: `Enter` confirms, `Esc` cancels
- `s` — save renames and continue the pipeline
- `q` (or `Ctrl+C`) — abort the pipeline (no summarization runs). Prompts for confirmation if you have unsaved renames.

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
    Transcribe(ctx context.Context, in Input) error
}

// internal/summarizer/summarizer.go
type Input struct {
    TranscriptFile string          // absolute path
    OutputFile     string          // file name (not a path)
    ObsidianVault  string          // empty when not writing to a vault
    ExtraArgs      []string        // already-split tokens to append to the tool's argv
    NoDefaults     bool            // skip the adapter's hardcoded extra flags
    ExtraPrompt    string          // --append-prompt text, appended after the base prompt
    BasePrompt     string          // base instruction; may contain {{transcript}}. Empty = config.DefaultBasePrompt
    Highlights     []time.Duration // user-marked moments (offsets from the start); e.g. Plaud highlights
    RequestTitle   bool            // ask the adapter to also produce a short meeting title
}

// Result reports values the summarizer produced alongside the notes file.
type Result struct {
    Title string // the short meeting title, when RequestTitle was set (best-effort; may be empty)
}

type Summarizer interface {
    Name() string
    Summarize(ctx context.Context, in Input) (Result, error)
}
```

Notes:

- The transcriber is given an absolute `OutputFile`; how it actually lands the transcript there (stdout redirect, a CLI `-o` flag, etc.) is the adapter's call. The `macwhisper` adapter redirects `mw`'s stdout to the file because `mw transcribe` has no output flag; the `whisperx` adapter passes `--output_dir` and renames the stem-derived file.
- Adapters should honor `ExtraArgs` (append after the adapter's defaults) and `NoDefaults` (skip the hardcoded extras when set). This is what makes `--transcriber-args` / `--summarizer-args` and the matching `--*-no-defaults` toggles work uniformly across adapters.
- `RequestTitle` / `Result.Title` are how `plaud run` gets a title to rename the recording with. Title capture is best-effort: an adapter returns an empty `Result.Title` with a nil error when it can't capture one, so a busted title never aborts a successful summarization. `Highlights`, when non-empty, are rendered into the prompt as important timestamps.
- Both `Input` structs leave room for future fields (model selection, system prompt overrides, etc.) to be added without breaking adapters.

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
