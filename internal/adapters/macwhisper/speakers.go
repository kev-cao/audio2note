package macwhisper

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"audio2note/internal/transcriber"
)

// MacWhisper's JSON export is an array of segment records, each with at least
// a "speaker" and a "text" field. We keep unknown fields as json.RawMessage so
// round-tripping through ApplyRenames preserves timestamps and anything else
// the export carries.

const (
	speakerKey = "speaker"
	textKey    = "text"
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

func readSegments(path string) ([]segment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var segs []segment
	if err := json.Unmarshal(data, &segs); err != nil {
		return nil, fmt.Errorf("parse %s as array of segments: %w", filepath.Base(path), err)
	}
	return segs, nil
}

func (MacWhisper) HasSpeakerLabels(path string) (bool, error) {
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return false, nil
	}
	segs, err := readSegments(path)
	if err != nil {
		// Not an array of objects → format we don't recognize → false (not an
		// error). A read error is different and bubbles up.
		if _, ok := err.(*json.SyntaxError); ok {
			return false, nil
		}
		if _, ok := err.(*json.UnmarshalTypeError); ok {
			return false, nil
		}
		// Fallback: also treat wrapped parse errors as "no labels", but real
		// I/O errors should surface.
		if strings.Contains(err.Error(), "parse ") {
			return false, nil
		}
		return false, err
	}
	if len(segs) == 0 {
		return false, nil
	}
	_, ok := segs[0][speakerKey]
	return ok, nil
}

func (MacWhisper) LoadSpeakers(path string) ([]transcriber.Speaker, error) {
	segs, err := readSegments(path)
	if err != nil {
		return nil, err
	}
	// Preserve first-appearance order via a parallel slice.
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

func (MacWhisper) ApplyRenames(path string, renames map[string]string) error {
	if len(renames) == 0 {
		return nil
	}
	segs, err := readSegments(path)
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
	out, err := json.MarshalIndent(segs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode segments: %w", err)
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
