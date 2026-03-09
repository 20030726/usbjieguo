package storage

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Storage struct {
	dir string
}

func New(dir string) *Storage {
	return &Storage{dir: dir}
}

// Init creates the save directory if it does not exist.
func (s *Storage) Init() error {
	return os.MkdirAll(s.dir, 0755)
}

// Dir returns the save directory path.
func (s *Storage) Dir() string { return s.dir }

// SetDir changes the save directory at runtime and ensures it exists.
func (s *Storage) SetDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create directory: %w", err)
	}
	s.dir = dir
	return nil
}

// Save writes data to the save directory.
// If the filename already exists, it appends (1), (2), ... to avoid overwrite.
func (s *Storage) Save(filename string, data []byte) (string, error) {
	dest := s.resolvePath(filename)
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return "", err
	}
	return filepath.Base(dest), nil
}

// SaveAndUnzip receives a zip archive and extracts it into a subdirectory of
// the save dir.  The subdirectory name is the zip filename without .zip.
// Returns the subdirectory name that was created.
func (s *Storage) SaveAndUnzip(zipFilename string, data []byte) (string, error) {
	// Derive directory name from zip filename (strip .zip extension).
	dirName := strings.TrimSuffix(filepath.Base(zipFilename), ".zip")

	// Resolve a non-conflicting destination directory.
	destDir := s.resolveDir(dirName)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("invalid zip: %w", err)
	}

	for _, f := range zr.File {
		// Sanitise path to prevent zip-slip.
		clean := filepath.Clean(filepath.FromSlash(f.Name))
		if strings.HasPrefix(clean, "..") {
			continue // skip dangerous paths
		}
		target := filepath.Join(destDir, clean)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return "", err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return "", err
		}
		out, err := os.Create(target)
		if err != nil {
			return "", err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return "", err
		}
		_, cpErr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if cpErr != nil {
			return "", cpErr
		}
	}
	return filepath.Base(destDir), nil
}

// resolveDir returns a non-conflicting full path for dirname.
func (s *Storage) resolveDir(dirname string) string {
	dest := filepath.Join(s.dir, dirname)
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}
	for i := 1; ; i++ {
		new := filepath.Join(s.dir, fmt.Sprintf("%s(%d)", dirname, i))
		if _, err := os.Stat(new); os.IsNotExist(err) {
			return new
		}
	}
}

// resolvePath returns a non-conflicting full path for filename.
func (s *Storage) resolvePath(filename string) string {
	// Use only the base name to prevent path traversal.
	filename = filepath.Base(filename)

	dest := filepath.Join(s.dir, filename)
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}

	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)

	for i := 1; ; i++ {
		newName := fmt.Sprintf("%s(%d)%s", base, i, ext)
		newPath := filepath.Join(s.dir, newName)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}
}
