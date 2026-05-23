package plaud

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListFiles_ParsesAndSendsAuth(t *testing.T) {
	var gotAuth string
	h := http.NewServeMux()
	h.HandleFunc("/file/simple/web", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"status":0,"data_file_list":[
			{"id":"abc123","filename":"meeting","fullname":"abc123.mp3","duration":226000,"is_trans":true},
			{"id":"def456","filename":"standup","fullname":"def456.ogg","duration":1500,"is_trans":false}
		]}`))
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	c := newAPIClient("test-token", ts.URL)
	files, err := c.ListFiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "bearer test-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "bearer test-token")
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	if files[0].ID != "abc123" || files[0].Filename != "meeting" {
		t.Errorf("files[0] = %+v", files[0])
	}
	if files[0].Extension != ".mp3" {
		t.Errorf("files[0].Extension = %q, want %q", files[0].Extension, ".mp3")
	}
	if !files[0].HasTranscript {
		t.Errorf("files[0].HasTranscript = false, want true")
	}
	if files[1].Extension != ".ogg" {
		t.Errorf("files[1].Extension = %q, want %q", files[1].Extension, ".ogg")
	}
	if files[1].HasTranscript {
		t.Errorf("files[1].HasTranscript = true, want false")
	}
	if want := 226000 * time.Millisecond; files[0].Duration != want {
		t.Errorf("files[0].Duration = %v, want %v", files[0].Duration, want)
	}
	if want := 1500 * time.Millisecond; files[1].Duration != want {
		t.Errorf("files[1].Duration = %v, want %v", files[1].Duration, want)
	}
}

func TestListFiles_NonZeroStatusErrors(t *testing.T) {
	h := http.NewServeMux()
	h.HandleFunc("/file/simple/web", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":401,"msg":"auth failed"}`))
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	c := newAPIClient("t", ts.URL)
	_, err := c.ListFiles(context.Background())
	if err == nil {
		t.Fatal("expected error on non-zero status")
	}
	if !strings.Contains(err.Error(), "auth failed") {
		t.Errorf("err should mention 'auth failed', got %v", err)
	}
}

func TestDownloadRecording_TwoStepAndNoAuthOnS3(t *testing.T) {
	var s3SawAuth string
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s3SawAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("FAKE_AUDIO_BYTES"))
	}))
	defer s3.Close()

	h := http.NewServeMux()
	h.HandleFunc("/file/temp-url/abc123", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"temp_url": s3.URL + "/audio.mp3?sig=xyz"})
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	c := newAPIClient("test-token", ts.URL)
	dest := filepath.Join(t.TempDir(), "out.mp3")
	got, err := c.DownloadRecording(context.Background(), "abc123", dest)
	if err != nil {
		t.Fatal(err)
	}
	wantAbs, _ := filepath.Abs(dest)
	if got != wantAbs {
		t.Errorf("returned path = %q, want %q", got, wantAbs)
	}
	if s3SawAuth != "" {
		t.Errorf("S3 saw Authorization header %q; should be empty", s3SawAuth)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "FAKE_AUDIO_BYTES" {
		t.Errorf("file contents = %q, want %q", data, "FAKE_AUDIO_BYTES")
	}
}

func TestGetFileDetails_ExtractsAndCaches(t *testing.T) {
	var hits int
	h := http.NewServeMux()
	h.HandleFunc("/file/detail/abc", func(w http.ResponseWriter, r *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": 0,
			"data": map[string]any{
				"content_list": []map[string]any{
					{"data_type": "transaction", "data_link": "https://s3.example/trans.gz"},
					{"data_type": "outline", "data_link": "https://s3.example/outline"},
					{"data_type": "mark_memo", "data_link": "https://s3.example/hl"},
					{"data_type": "auto_sum_note", "data_link": "https://s3.example/sum"},
				},
			},
		})
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	c := newAPIClient("t", ts.URL)
	fd, err := c.getFileDetails(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if fd.TranscriptURL != "https://s3.example/trans.gz" {
		t.Errorf("TranscriptURL = %q", fd.TranscriptURL)
	}
	if fd.HighlightsURL != "https://s3.example/hl" {
		t.Errorf("HighlightsURL = %q", fd.HighlightsURL)
	}

	// Second call: should hit the cache.
	_, _ = c.getFileDetails(context.Background(), "abc")
	if hits != 1 {
		t.Errorf("API hit count = %d, want 1 (second call should be cached)", hits)
	}
}

func TestDownloadTranscript_DecompressesGzip(t *testing.T) {
	plain := []byte(`[{"speaker":"Speaker 1","text":"hello"},{"speaker":"Speaker 2","text":"hi"}]`)

	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gw := gzip.NewWriter(w)
		_, _ = gw.Write(plain)
		_ = gw.Close()
	}))
	defer s3.Close()

	h := http.NewServeMux()
	h.HandleFunc("/file/detail/abc", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": 0,
			"data": map[string]any{
				"content_list": []map[string]any{
					{"data_type": "transaction", "data_link": s3.URL + "/trans.json.gz"},
				},
			},
		})
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	c := newAPIClient("t", ts.URL)
	dest := filepath.Join(t.TempDir(), "trans.json")
	got, err := c.DownloadTranscript(context.Background(), "abc", dest)
	if err != nil {
		t.Fatal(err)
	}
	wantAbs, _ := filepath.Abs(dest)
	if got != wantAbs {
		t.Errorf("returned path = %q, want %q", got, wantAbs)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != string(plain) {
		t.Errorf("decompressed file = %q, want %q", data, plain)
	}
}

func TestDownloadTranscript_NoTranscriptErrors(t *testing.T) {
	h := http.NewServeMux()
	h.HandleFunc("/file/detail/abc", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": 0,
			"data": map[string]any{
				"content_list": []map[string]any{
					{"data_type": "outline", "data_link": "https://s3.example/outline"},
				},
			},
		})
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	c := newAPIClient("t", ts.URL)
	_, err := c.DownloadTranscript(context.Background(), "abc", filepath.Join(t.TempDir(), "x.json"))
	if err == nil || !strings.Contains(err.Error(), "no transcript") {
		t.Errorf("want 'no transcript' error, got %v", err)
	}
}

func TestGetHighlights_ExtractsTimestamps(t *testing.T) {
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"timestamp":961640,"title":"first","content":"..."},
			{"timestamp":1650820,"title":"second"},
			{"timestamp":1892040}
		]`))
	}))
	defer s3.Close()

	h := http.NewServeMux()
	h.HandleFunc("/file/detail/abc", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": 0,
			"data": map[string]any{
				"content_list": []map[string]any{
					{"data_type": "mark_memo", "data_link": s3.URL + "/hl"},
				},
			},
		})
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	c := newAPIClient("t", ts.URL)
	hl, err := c.GetHighlights(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	want := []Highlight{
		Highlight(961640 * time.Millisecond),
		Highlight(1650820 * time.Millisecond),
		Highlight(1892040 * time.Millisecond),
	}
	if len(hl) != len(want) {
		t.Fatalf("got %d highlights, want %d", len(hl), len(want))
	}
	for i, h := range hl {
		if h != want[i] {
			t.Errorf("hl[%d] = %v, want %v", i, time.Duration(h), time.Duration(want[i]))
		}
	}
}

func TestGetHighlights_MissingURLReturnsEmpty(t *testing.T) {
	h := http.NewServeMux()
	h.HandleFunc("/file/detail/abc", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": 0,
			"data":   map[string]any{"content_list": []map[string]any{}},
		})
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	c := newAPIClient("t", ts.URL)
	hl, err := c.GetHighlights(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if hl != nil {
		t.Errorf("want nil highlights, got %v", hl)
	}
}

func TestValidate_OK(t *testing.T) {
	var gotAuth string
	h := http.NewServeMux()
	h.HandleFunc("/user/me", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"status":0,"msg":"success","data":{}}`))
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	c := newAPIClient("real-token", ts.URL)
	if err := c.Validate(context.Background()); err != nil {
		t.Fatalf("Validate err = %v", err)
	}
	if gotAuth != "bearer real-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "bearer real-token")
	}
}

func TestValidate_Unauthorized(t *testing.T) {
	h := http.NewServeMux()
	h.HandleFunc("/user/me", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":401,"msg":"unauthorized"}`))
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	c := newAPIClient("expired-token", ts.URL)
	err := c.Validate(context.Background())
	if err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestValidate_NonZeroStatus(t *testing.T) {
	h := http.NewServeMux()
	h.HandleFunc("/user/me", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":1,"msg":"token expired"}`))
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	c := newAPIClient("expired-token", ts.URL)
	err := c.Validate(context.Background())
	if err == nil {
		t.Fatal("expected error on non-zero envelope status")
	}
	if !strings.Contains(err.Error(), "token expired") {
		t.Errorf("err should include 'token expired', got %v", err)
	}
}

func TestNewClient_NoToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := NewClient()
	if err == nil {
		t.Fatal("expected error when no token saved")
	}
	if !errors.Is(err, ErrMissing) {
		t.Errorf("err should wrap ErrMissing, got %v", err)
	}
}
