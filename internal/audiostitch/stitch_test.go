package audiostitch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStitch_RejectsSingleInput(t *testing.T) {
	err := Stitch(context.Background(), []string{"only.mp3"}, "/tmp/out.mp3")
	if err == nil || !strings.Contains(err.Error(), "at least 2 inputs") {
		t.Fatalf("want error about needing 2+ inputs; got %v", err)
	}
}

func TestStitch_RejectsMixedExtensions(t *testing.T) {
	err := Stitch(context.Background(), []string{"a.mp3", "b.ogg"}, "/tmp/out.mp3")
	if err == nil || !strings.Contains(err.Error(), "mixed formats") {
		t.Fatalf("want error about mixed formats; got %v", err)
	}
}

func TestStitch_ConcatListEscapesQuotes(t *testing.T) {
	dir := t.TempDir()
	weird := filepath.Join(dir, "it's mine.mp3")
	plain := filepath.Join(dir, "plain.mp3")
	path, err := writeConcatList([]string{weird, plain})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	wantEscaped := `'\''`
	if !strings.Contains(got, wantEscaped) {
		t.Errorf("concat list missing escaped quote sequence; got:\n%s", got)
	}
	if !strings.Contains(got, "plain.mp3") {
		t.Errorf("concat list missing second input; got:\n%s", got)
	}
}

// TestStitch_IntegrationWithFFmpeg runs a real stitch over two tiny MP3
// fixtures generated on the fly via ffmpeg. Skipped when ffmpeg isn't on PATH.
func TestStitch_IntegrationWithFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mp3")
	b := filepath.Join(dir, "b.mp3")
	out := filepath.Join(dir, "out.mp3")
	for _, path := range []string{a, b} {
		// 0.5s of silence at 44.1kHz mono encoded as mp3.
		cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "anullsrc=channel_layout=mono:sample_rate=44100",
			"-t", "0.5", "-c:a", "libmp3lame", "-y", path)
		if err := cmd.Run(); err != nil {
			t.Skipf("ffmpeg can't synthesize fixture (missing libmp3lame?): %v", err)
		}
	}
	if err := Stitch(context.Background(), []string{a, b}, out); err != nil {
		t.Fatalf("Stitch: %v", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	infoA, _ := os.Stat(a)
	infoB, _ := os.Stat(b)
	// Stream copy concat keeps the bytes; the output should be at least the
	// larger of the two inputs (typically ~sum minus container overhead).
	if info.Size() < infoA.Size() || info.Size() < infoB.Size() {
		t.Errorf("stitched output (%d bytes) is smaller than inputs (%d, %d)", info.Size(), infoA.Size(), infoB.Size())
	}
}
