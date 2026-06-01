package plaud

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	defaultHost    = "https://api.plaud.ai"
	defaultTimeout = 60 * time.Second
)

// fileDetails caches the presigned URLs we extract from /file/detail/{id}.
type fileDetails struct {
	TranscriptURL string
	HighlightsURL string
}

type apiClient struct {
	token string
	host  string
	http  *http.Client

	detailsMu sync.Mutex
	details   map[ID]fileDetails
}

// NewClient builds a Client backed by the real Plaud API. It loads the bearer
// token from ~/.plaud/token and returns an error if the token is not present.
func NewClient() (Client, error) {
	token, err := LoadToken()
	if err != nil {
		return nil, fmt.Errorf("load plaud token: %w", err)
	}
	return newAPIClient(token, defaultHost), nil
}

// newAPIClient is unexported so tests can point the client at a httptest URL.
func newAPIClient(token, host string) *apiClient {
	return &apiClient{
		token:   token,
		host:    host,
		http:    &http.Client{Timeout: defaultTimeout},
		details: map[ID]fileDetails{},
	}
}

func (c *apiClient) Validate(ctx context.Context) error {
	var ignore struct{}
	return c.doAPI(ctx, "GET", "/user/me", &ignore)
}

func (c *apiClient) ListFiles(ctx context.Context) ([]File, error) {
	var resp struct {
		DataFileList []struct {
			ID       ID     `json:"id"`
			Filename string `json:"filename"`
			Fullname string `json:"fullname"`
			Duration int64  `json:"duration"`
			IsTrans  bool   `json:"is_trans"`
		} `json:"data_file_list"`
	}
	if err := c.doAPI(ctx, "GET", "/file/simple/web", &resp); err != nil {
		return nil, err
	}
	out := make([]File, len(resp.DataFileList))
	for i, f := range resp.DataFileList {
		out[i] = File{
			ID:            f.ID,
			Filename:      f.Filename,
			Extension:     filepath.Ext(f.Fullname),
			Duration:      time.Duration(f.Duration) * time.Millisecond,
			HasTranscript: f.IsTrans,
		}
	}
	return out, nil
}

func (c *apiClient) DownloadRecording(
	ctx context.Context, id ID, outputFilePath string,
) (string, error) {
	var resp struct {
		TempURL string `json:"temp_url"`
	}
	if err := c.doAPI(ctx, "GET", "/file/temp-url/"+string(id), &resp); err != nil {
		return "", err
	}
	if resp.TempURL == "" {
		return "", fmt.Errorf("plaud: empty temp_url for %s", id)
	}
	if err := c.downloadToFile(ctx, resp.TempURL, outputFilePath); err != nil {
		return "", err
	}
	return filepath.Abs(outputFilePath)
}

func (c *apiClient) DownloadTranscript(
	ctx context.Context, id ID, outputFilePath string,
) (string, error) {
	fd, err := c.getFileDetails(ctx, id)
	if err != nil {
		return "", err
	}
	if fd.TranscriptURL == "" {
		return "", fmt.Errorf("plaud: no transcript available for %s", id)
	}
	if err := c.downloadGzipToFile(ctx, fd.TranscriptURL, outputFilePath); err != nil {
		return "", err
	}
	return filepath.Abs(outputFilePath)
}

func (c *apiClient) RenameFile(ctx context.Context, id ID, newName string) error {
	body := map[string]any{
		"filename": newName,
		// hasTitleEdit tells Plaud the rename is user-initiated; without it
		// the API ignores the filename change. The shape is what the web
		// client sends — keep it verbatim.
		"extra_data": map[string]any{
			"actionData": map[string]any{
				"hasTitleEdit": true,
			},
		},
	}
	var ignore struct{}
	return c.doAPIWithBody(ctx, "PATCH", "/file/"+string(id), body, &ignore)
}

func (c *apiClient) GetHighlights(ctx context.Context, id ID) ([]Highlight, error) {
	fd, err := c.getFileDetails(ctx, id)
	if err != nil {
		return nil, err
	}
	if fd.HighlightsURL == "" {
		return nil, nil
	}
	body, err := c.fetchURL(ctx, fd.HighlightsURL)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Timestamp int64 `json:"timestamp"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse highlights: %w", err)
	}
	out := make([]Highlight, len(raw))
	for i, r := range raw {
		out[i] = Highlight(time.Duration(r.Timestamp) * time.Millisecond)
	}
	return out, nil
}

func (c *apiClient) getFileDetails(ctx context.Context, id ID) (fileDetails, error) {
	c.detailsMu.Lock()
	defer c.detailsMu.Unlock()
	if fd, ok := c.details[id]; ok {
		return fd, nil
	}
	var resp struct {
		Data struct {
			ContentList []struct {
				DataType string `json:"data_type"`
				DataLink string `json:"data_link"`
			} `json:"content_list"`
		} `json:"data"`
	}
	if err := c.doAPI(ctx, "GET", "/file/detail/"+string(id), &resp); err != nil {
		return fileDetails{}, err
	}
	var fd fileDetails
	for _, item := range resp.Data.ContentList {
		switch item.DataType {
		case "transaction":
			fd.TranscriptURL = item.DataLink
		case "mark_memo":
			// `mark_memo` is the user-authored highlights file. The
			// `high_light` data_type is a separate concept that only exists
			// when Plaud performed the transcription itself, so we don't
			// match on it here.
			fd.HighlightsURL = item.DataLink
		}
	}
	c.details[id] = fd
	return fd, nil
}

// doAPI sends an authenticated request to the Plaud API and decodes the JSON
// response into dst. Errors on non-2xx HTTP or non-zero `status` in the
// response envelope.
func (c *apiClient) doAPI(ctx context.Context, method, path string, dst any) error {
	return c.doAPIWithBody(ctx, method, path, nil, dst)
}

// doAPIWithBody is doAPI plus a JSON request body. nil body skips the body
// (GETs use this).
func (c *apiClient) doAPIWithBody(
	ctx context.Context, method, path string, body any, dst any,
) error {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("plaud %s %s: marshal body: %w", method, path, err)
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.host+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("plaud %s %s: http %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	var envelope struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Status != 0 {
		return fmt.Errorf("plaud %s %s: status=%d msg=%q", method, path, envelope.Status, envelope.Msg)
	}
	return json.Unmarshal(respBody, dst)
}

// fetchURL GETs a presigned URL (no auth header) and returns the full body.
func (c *apiClient) fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("download %s: http %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// downloadToFile streams a presigned URL body to dest. No auth header — the
// presigned URL carries its own credentials in the query string.
func (c *apiClient) downloadToFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("download %s: http %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// downloadGzipToFile streams a presigned URL body through gzip and writes the
// decompressed bytes to dest.
func (c *apiClient) downloadGzipToFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("download %s: http %d", url, resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("gunzip %s: %w", url, err)
	}
	defer gz.Close()
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, gz)
	return err
}
