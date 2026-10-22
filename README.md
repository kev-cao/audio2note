# audio2note

A small Go CLI that turns audio into meeting notes by chaining a transcriber and an AI summarizer.

```
audio file ──► transcriber ──► transcript.json ──► summarizer ──► notes.md
```

Audio comes from either local files (`audio2note run <file>...`) or your [Plaud](https://www.plaud.ai/) library (`audio2note plaud run`). Each side of the pipeline is a pluggable adapter:

| Role | Adapters |
| --- | --- |
| Transcriber | `macwhisper` (MacWhisper `mw` CLI), `whisperx` |
| Summarizer | `claudecode` (`claude`), `opencode` |
| Audio preprocessing (optional) | `resemble-enhance` |

Adding an adapter is a one-file change — see [Adding an adapter](#adding-an-adapter).

## Prerequisites

You need Go, one transcriber, and one summarizer; everything else is feature-specific. The default `audio2note run <file>` uses **macwhisper + claudecode**, so install those two for the quickest start.

- **Go 1.22+**
- **A transcriber:** [MacWhisper](https://goodsnooze.gumroad.com/l/macwhisper) (`mw`, default) or [WhisperX](https://github.com/m-bain/whisperX) (`whisperx`)
- **A summarizer:** [Claude Code](https://docs.claude.com/en/docs/claude-code) (`claude`, default) or [OpenCode](https://opencode.ai/) (`opencode`)
- [ffmpeg](https://ffmpeg.org/) — only to stitch multiple files in one run
- [resemble-enhance](https://github.com/resemble-ai/resemble-enhance) + `git-lfs` — only for `--denoise` / `--enhance-audio`
- A [Plaud](https://www.plaud.ai/) account — only for the `plaud` commands

> **whisperx** defaults to `--model small --device cuda --diarize`, so it expects a CUDA GPU and HuggingFace auth (`$HF_TOKEN` or `hf auth login`) for diarization. No GPU? Add `--transcriber-args "--device cpu"`.

## Build

```sh
go build -o bin/audio2note .
```

## Usage

```sh
audio2note run meeting.mp3            # transcribe + summarize a local file
audio2note run part1.mp3 part2.mp3    # multiple files are stitched in order (needs ffmpeg)
audio2note plaud run                  # pick recording(s) from your Plaud library instead
```

By default macwhisper writes the transcript to a temp file and claudecode writes the notes (choosing the filename unless you pass `--output-file`). See [Flags](#flags) for the knobs and [Configuration](#configuration) for persistent defaults.

## Plaud

`audio2note plaud run` pulls recordings straight from your Plaud library — no manual export. It runs the same pipeline as local files (all shared [flags](#flags) apply) and additionally:

- lets you pick one or more recordings (multi-select; recordings with an existing Plaud transcript are marked);
- stitches a multi-recording selection into one timeline;
- passes the moments you highlighted in the Plaud app to the summarizer as important timestamps;
- renames the recording to the summarizer's title after a successful run (best-effort; disable with `--no-rename`).

With `--summarize-only` it uses the transcript Plaud already has instead of downloading audio.

### Setup

The `plaud` commands need a bearer token from the Plaud web app (there's no official API):

1. Sign in at [web.plaud.ai](https://web.plaud.ai).
2. In the browser dev tools, copy the `pid_tokenstr` value from localStorage.
3. Run `audio2note plaud auth setup` and paste it (stored as `plaudToken` in the [config file](#configuration)).

`audio2note plaud auth validate` checks the token; re-run `setup` when it expires.

## Flags

Shared by `run` and `plaud run` (`--no-rename` is `plaud run`-only):

| Flag | Default | Behavior |
| --- | --- | --- |
| `--transcriber` | `macwhisper` | Transcriber adapter (`macwhisper`, `whisperx`). |
| `--summarizer` | `claudecode` | Summarizer adapter (`claudecode`, `opencode`). |
| `--transcript-file <path>` | temp file | Where the transcript is written / read from. |
| `--obsidian` | off | Write notes to the `obsidianVault` set in [config](#configuration). |
| `--obsidian-vault <path>` | unset | Vault path for this run (implies `--obsidian`). |
| `--output-file <name>` | _(summarizer chooses)_ | Force the notes filename. |
| `--transcribe-only` | off | Transcribe and stop. |
| `--summarize-only` | off | Summarize an existing transcript (no audio needed). |
| `--transcriber-args <string>` | `""` | Extra shell-quoted args forwarded to the transcriber CLI. |
| `--summarizer-args <string>` | `""` | Extra shell-quoted args forwarded to the summarizer CLI. |
| `--transcriber-no-defaults` | off | Skip the transcriber adapter's hardcoded extra flags. |
| `--summarizer-no-defaults` | off | Skip the summarizer adapter's hardcoded extra flags. |
| `--append-prompt <string>` | `""` | Text appended after the base prompt for one run (change the base prompt via `basePrompt` in [config](#configuration)). |
| `--edit-speaker-labels` | off | Rename speaker labels interactively before summarizing (see [below](#speaker-label-editing)). |
| `--denoise` | off | Denoise audio with resemble-enhance before transcription (fast, non-generative). Excludes `--enhance-audio`. |
| `--enhance-audio` | off | **Experimental.** Full resemble-enhance restoration (generative); can alter words and hurt accuracy — prefer `--denoise`. |
| `--no-rename` _(plaud)_ | off | Don't auto-rename the Plaud recording after the run. |

Repeated flags are last-wins, so `--transcriber-args` / `--summarizer-args` override an adapter's hardcoded defaults (those live in `internal/adapters/<name>/<name>.go`).

## Configuration

Persistent defaults live in `~/.audio2note/config.json` (optional; any missing key falls back to its default). Precedence is **explicit flag → config → built-in default**. Manage it with the `config` subcommands:

```sh
audio2note config get [key]          # show effective value(s)
audio2note config set <key> <value>  # set a key (validated; writes only what you set)
audio2note config help [key]         # describe key(s), with valid options for restricted keys
```

| Key | Default | Notes |
| --- | --- | --- |
| `defaultTranscriber` | `macwhisper` | Default `--transcriber` (`macwhisper`, `whisperx`). |
| `defaultSummarizer` | `claudecode` | Default `--summarizer` (`claudecode`, `opencode`). |
| `basePrompt` | _(see below)_ | The summarizer's opening instruction. |
| `obsidianVault` | unset | Default vault path; when set, notes go to the vault without a flag. |
| `plaudToken` | unset | Plaud bearer token. Set via [`plaud auth setup`](#setup), not `config set`; shown masked. |

`basePrompt` replaces the summarizer's opening instruction (default: `Read through the transcript located at {{transcript}} and generate a summary.`). `{{transcript}}` is substituted with the transcript path; the structural parts (output filename, vault, highlights, title) are always appended after it, as is any `--append-prompt` text.

## Examples

```sh
# Notes into your Obsidian vault
audio2note run --obsidian meeting.mp3

# Use different adapters for one run
audio2note run --transcriber whisperx --summarizer opencode meeting.mp3

# Transcribe now, summarize later
audio2note run --transcribe-only meeting.mp3
audio2note run --summarize-only --transcript-file /tmp/audio2note-transcript.json

# Shape the summary for one run
audio2note run --append-prompt "Bullet points grouped by speaker, under 200 words." meeting.mp3

# Rename speaker labels before summarizing (interactive)
audio2note run --edit-speaker-labels meeting.mp3
```

## Speaker-label editing

`--edit-speaker-labels` opens an interactive prompt before summarization: page through each speaker's quotes, rename them, and save (or abort). Requires a transcriber that emits speaker labels (currently `macwhisper` with a `.json` transcript).

## Adding an adapter

1. Create `internal/adapters/<name>/<name>.go`.
2. Implement `transcriber.Transcriber` or `summarizer.Summarizer`.
3. Register it in `init()`: `func init() { transcriber.Register(MyAdapter{}) }`.
4. Add a blank import to `cmd/root.go`: `_ "audio2note/internal/adapters/<name>"`.

It's then selectable via `--transcriber <name>` / `--summarizer <name>`.

### Interfaces

```go
// internal/transcriber/transcriber.go
type Input struct {
    AudioFile  string   // absolute path
    OutputFile string   // absolute path
    ExtraArgs  []string // tokens to append to the tool's argv (--transcriber-args)
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
    ExtraArgs      []string        // tokens to append to the tool's argv (--summarizer-args)
    NoDefaults     bool            // skip the adapter's hardcoded extra flags
    ExtraPrompt    string          // --append-prompt text, appended after the base prompt
    BasePrompt     string          // base instruction; may contain {{transcript}}. Empty = default
    Highlights     []time.Duration // user-marked moments (e.g. Plaud highlights)
    RequestTitle   bool            // also produce a short meeting title in Result.Title
}

type Result struct {
    Title string // meeting title when RequestTitle was set (best-effort; may be empty)
}

type Summarizer interface {
    Name() string
    Summarize(ctx context.Context, in Input) (Result, error)
}
```

Adapters land the transcript at the absolute `OutputFile` however suits the tool (macwhisper redirects stdout; whisperx uses `--output_dir` then renames), and should honor `ExtraArgs` / `NoDefaults`. Title and highlight handling are best-effort — a failure never aborts a successful run.

### Optional capability: speaker-label editing

Transcribers whose output can be parsed and rewritten for speaker editing implement `transcriber.SpeakerLabelEditor` (`HasSpeakerLabels` / `LoadSpeakers` / `ApplyRenames`); `--edit-speaker-labels` type-asserts into it and errors clearly on transcribers that don't. `macwhisper` implements it for JSON transcripts, preserving per-segment fields via `json.RawMessage`.
