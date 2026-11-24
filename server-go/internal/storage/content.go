package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ContentManifest pins game scripts and assets by SHA-256.
type ContentManifest struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
}

func validateContentPath(relative string) error {
	if relative == "" || filepath.IsAbs(relative) {
		return fmt.Errorf("invalid content path %q", relative)
	}
	cleaned := filepath.Clean(relative)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("content path escapes root: %q", relative)
	}
	return nil
}

// LoadContentManifest reads and validates a content manifest.
func LoadContentManifest(path string) (ContentManifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return ContentManifest{}, fmt.Errorf("open content manifest: %w", err)
	}
	defer file.Close()
	var manifest ContentManifest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return ContentManifest{}, fmt.Errorf("decode content manifest: %w", err)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return ContentManifest{}, errors.New("content manifest version is empty")
	}
	if len(manifest.Files) == 0 {
		return ContentManifest{}, errors.New("content manifest has no files")
	}
	for relative, checksum := range manifest.Files {
		if err := validateContentPath(relative); err != nil {
			return ContentManifest{}, err
		}
		if len(checksum) != sha256.Size*2 {
			return ContentManifest{}, fmt.Errorf("invalid sha256 for %s", relative)
		}
		if _, err := hex.DecodeString(checksum); err != nil {
			return ContentManifest{}, fmt.Errorf("invalid sha256 for %s: %w", relative, err)
		}
	}
	return manifest, nil
}

// VerifyContent checks every manifest entry below root.
func VerifyContent(root string, manifest ContentManifest) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(manifest.Files))
	for relative := range manifest.Files {
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	for _, relative := range paths {
		if err := validateContentPath(relative); err != nil {
			return err
		}
		file, err := os.Open(filepath.Join(root, relative))
		if err != nil {
			return fmt.Errorf("open content %s: %w", relative, err)
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("hash content %s: %w", relative, copyErr)
		}
		if closeErr != nil {
			return closeErr
		}
		actual := hex.EncodeToString(hash.Sum(nil))
		if !strings.EqualFold(actual, manifest.Files[relative]) {
			return fmt.Errorf("content hash mismatch for %s", relative)
		}
	}
	return nil
}
