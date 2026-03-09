package client

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ProgressFunc is called with (sent, total) bytes during an upload.
// total is 0 if the file size is unknown.
type ProgressFunc func(sent, total int64)

// Client sends files to a usbjieguo server.
type Client struct {
	target     string
	httpClient *http.Client
}

// New creates a Client targeting host:port.
func New(target string) *Client {
	return &Client{
		target: target,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type uploadResponse struct {
	Status   string `json:"status"`
	Filename string `json:"filename"`
}

// Send uploads the file at filePath to the target server.
// Returns the filename as saved on the server (may differ if renamed to avoid conflict).
func (c *Client) Send(filePath string) (string, error) {
	// 1. Validate file.
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory, not a file", filePath)
	}

	// 2. Open file.
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	// 3. Build multipart/form-data body.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", fmt.Errorf("failed to create form field: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	writer.Close()

	// 4. Send POST /upload.
	url := fmt.Sprintf("http://%s/upload", c.target)
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("target not reachable: %w", err)
	}
	defer resp.Body.Close()

	// 5. Handle non-200 responses.
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server returned %d: %s", resp.StatusCode, string(data))
	}

	// 6. Parse success response.
	var result uploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("invalid response from server: %w", err)
	}
	if result.Status != "ok" {
		return "", fmt.Errorf("upload failed: %s", result.Status)
	}

	return result.Filename, nil
}

// SendDir zips dirPath in memory and uploads it as "dirname.zip" with the
// X-Is-Dir header so the server knows to unzip it on the other side.
func (c *Client) SendDir(dirPath string) error {
	info, err := os.Stat(dirPath)
	if err != nil {
		return fmt.Errorf("path not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory, use Send instead", dirPath)
	}

	zipData, err := zipDir(dirPath)
	if err != nil {
		return fmt.Errorf("zip error: %w", err)
	}

	zipName := filepath.Base(dirPath) + ".zip"

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", zipName)
	if err != nil {
		return fmt.Errorf("failed to create form field: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(zipData)); err != nil {
		return fmt.Errorf("failed to write zip: %w", err)
	}
	writer.Close()

	url := fmt.Sprintf("http://%s/upload", c.target)
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Is-Dir", "true")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("target not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(data))
	}

	var result uploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("invalid response from server: %w", err)
	}
	if result.Status != "ok" {
		return fmt.Errorf("upload failed: %s", result.Status)
	}
	return nil
}

// zipDir walks src and writes all files into an in-memory zip archive.
func zipDir(src string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	base := filepath.Dir(src) // write paths relative to parent of src
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		// Use forward slashes in the zip (cross-platform).
		rel = filepath.ToSlash(rel)

		if info.IsDir() {
			// Add directory entry (must end with "/").
			_, err = zw.Create(rel + "/")
			return err
		}

		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
func (c *Client) SendWithProgress(filePath string, progress ProgressFunc) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("%q is a directory, not a file", filePath)
	}
	total := info.Size()

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("failed to create form field: %w", err)
	}

	pr := &progressReader{r: f, total: total, fn: progress}
	if _, err := io.Copy(part, pr); err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	writer.Close()

	url := fmt.Sprintf("http://%s/upload", c.target)
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("target not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(data))
	}

	var result uploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("invalid response from server: %w", err)
	}
	if result.Status != "ok" {
		return fmt.Errorf("upload failed: %s", result.Status)
	}
	return nil
}

// progressReader wraps an io.Reader and calls fn after each Read.
type progressReader struct {
	r    io.Reader
	sent int64
	total int64
	fn   ProgressFunc
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.sent += int64(n)
	if pr.fn != nil {
		pr.fn(pr.sent, pr.total)
	}
	return n, err
}
