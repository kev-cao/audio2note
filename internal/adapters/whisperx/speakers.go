package whisperx

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"audio2note/internal/transcriber"
)

// whisperx's JSON output has the shape:
//
//	{
//	  "segments": [
//	    {"speaker": "SPEAKER_00", "text": "...", "start": 0.0, "end": 1.5, ...},
//	    ...
//	  ],
//	  "language": "en",
//	  ...
//	}
//
// We round-trip the top-level object and each segment through
// map[string]json.RawMessage so unrelated fields (language, word-level
// alignment, etc.) survive an ApplyRenames rewrite.

const (
	segmentsKey     = "segments"
	wordSegmentsKey = "word_segments"
	speakerKey      = "speaker"
	textKey         = "text"
	languageKey     = "language"
)

type segment map[string]json.RawMessage

func (s segment) speaker() (string, bool) {
	raw, ok := s[speakerKey]
	if !ok {
		return "", false
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", false
	}
	return v, true
}

func (s segment) text() string {
	raw, ok := s[textKey]
	if !ok {
		return ""
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	return v
}

type document map[string]json.RawMessage

func readDocument(path string) (document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d document
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parse %s as JSON object: %w", filepath.Base(path), err)
	}
	return d, nil
}

func segmentsFrom(d document) ([]segment, error) {
	raw, ok := d[segmentsKey]
	if !ok {
		return nil, nil
	}
	var segs []segment
	if err := json.Unmarshal(raw, &segs); err != nil {
		return nil, fmt.Errorf("parse segments: %w", err)
	}
	return segs, nil
}

func (WhisperX) HasSpeakerLabels(path string) (bool, error) {
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return false, nil
	}
	d, err := readDocument(path)
	if err != nil {
		// Parse failure → format we don't recognize → false (not an error)
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return false, nil
		}
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			return false, nil
		}
		if strings.Contains(err.Error(), "parse ") {
			return false, nil
		}
		return false, err
	}
	segs, err := segmentsFrom(d)
	if err != nil || len(segs) == 0 {
		return false, nil
	}
	_, ok := segs[0][speakerKey]
	return ok, nil
}

func (WhisperX) LoadSpeakers(path string) ([]transcriber.Speaker, error) {
	d, err := readDocument(path)
	if err != nil {
		return nil, err
	}
	segs, err := segmentsFrom(d)
	if err != nil {
		return nil, err
	}
	indexByName := map[string]int{}
	var speakers []transcriber.Speaker
	for _, seg := range segs {
		name, ok := seg.speaker()
		if !ok {
			continue
		}
		quote := transcriber.Quote{Text: seg.text()}
		if idx, seen := indexByName[name]; seen {
			speakers[idx].Quotes = append(speakers[idx].Quotes, quote)
			continue
		}
		indexByName[name] = len(speakers)
		speakers = append(speakers, transcriber.Speaker{
			Name:   name,
			Quotes: []transcriber.Quote{quote},
		})
	}
	return speakers, nil
}

func (WhisperX) ApplyRenames(path string, renames map[string]string) error {
	if len(renames) == 0 {
		return nil
	}
	d, err := readDocument(path)
	if err != nil {
		return err
	}
	segs, err := segmentsFrom(d)
	if err != nil {
		return err
	}
	for _, seg := range segs {
		name, ok := seg.speaker()
		if !ok {
			continue
		}
		newName, ok := renames[name]
		if !ok {
			continue
		}
		encoded, err := json.Marshal(newName)
		if err != nil {
			return fmt.Errorf("encode speaker name: %w", err)
		}
		seg[speakerKey] = encoded
	}
	rawSegs, err := json.Marshal(segs)
	if err != nil {
		return fmt.Errorf("encode segments: %w", err)
	}
	d[segmentsKey] = rawSegs
	out, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("encode document: %w", err)
	}
	return atomicWrite(path, out)
}

// atomicWrite writes data to a sibling temp file and renames it over path.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("rename over %s: %w", path, err)
	}
	return nil
}

// wordSeg is one whisperx word-level token with its speaker assignment.
// whisperx writes words under the "word" key (not "text").
type wordSeg struct {
	Text    string  `json:"word"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Speaker string  `json:"speaker"`
}

// mergedSegment is one run of words attributed to a single speaker. It's the
// segment shape downstream code expects (matches the old --no_align output,
// minus avg_logprob).
type mergedSegment struct {
	Text    string  `json:"text"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Speaker string  `json:"speaker"`
}

// transform rewrites whisperx's raw output JSON into the adapter's expected
// segment format. Without --no_align whisperx performs word-level diarization:
// each word carries its own speaker label, so attributing whole segments risks
// misattribution when people talk over each other. transform flattens those
// word labels and merges consecutive same-speaker words into a single segment.
// It's a no-op when the file is missing or doesn't contain word-level speaker
// data (e.g. --no_align output or a non-JSON format in NoDefaults mode).
func transform(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	d, err := readDocument(path)
	if err != nil {
		// Not a JSON document we recognize; leave it untouched.
		return nil
	}
	words, ok := wordsFrom(d)
	if !ok || !hasSpeaker(words) {
		return nil
	}

	merged := mergeWords(words)
	out := document{}
	if lang, ok := d[languageKey]; ok {
		out[languageKey] = lang
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("encode segments: %w", err)
	}
	out[segmentsKey] = raw
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encode document: %w", err)
	}
	return atomicWrite(path, data)
}

// wordsFrom returns whisperx's word-level tokens. They exist whenever forced
// alignment ran (i.e. --no_align was NOT set); the top-level word_segments list
// is exactly the flattening of every segment's words.
func wordsFrom(d document) ([]wordSeg, bool) {
	raw, ok := d[wordSegmentsKey]
	if !ok {
		return nil, false
	}
	var words []wordSeg
	if err := json.Unmarshal(raw, &words); err != nil {
		return nil, false
	}
	return words, true
}

func hasSpeaker(words []wordSeg) bool {
	for _, w := range words {
		if w.Speaker != "" {
			return true
		}
	}
	return false
}

// mergeWords groups consecutive words by speaker. A word with no speaker is
// folded into the surrounding run (the previous speaker, or the first speaker
// seen for a leading run) so no transcribed text is dropped.
func mergeWords(words []wordSeg) []mergedSegment {
	var runs []mergedSegment
	var cur []wordSeg
	lastSpeaker := ""
	for _, w := range words {
		if w.Speaker == "" {
			w.Speaker = lastSpeaker
		}
		if w.Speaker == "" {
			// Leading words before any speaker is known; attach to the first
			// run below rather than dropping them.
			cur = append(cur, w)
			continue
		}
		if n := len(cur); n > 0 {
			if sp := cur[n-1].Speaker; sp != "" && sp != w.Speaker {
				runs = append(runs, buildSegment(cur))
				cur = nil
			}
		}
		for i := range cur {
			if cur[i].Speaker == "" {
				cur[i].Speaker = w.Speaker
			}
		}
		cur = append(cur, w)
		lastSpeaker = w.Speaker
	}
	if len(cur) > 0 {
		runs = append(runs, buildSegment(cur))
	}
	return runs
}

func buildSegment(words []wordSeg) mergedSegment {
	texts := make([]string, len(words))
	for i, w := range words {
		texts[i] = w.Text
	}
	return mergedSegment{
		Text:    " " + strings.Join(texts, " "),
		Start:   words[0].Start,
		End:     words[len(words)-1].End,
		Speaker: words[0].Speaker,
	}
}
