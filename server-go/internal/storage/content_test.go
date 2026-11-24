package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestContentManifestVerification(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	contentPath := filepath.Join(root, "script", "npc.lua")
	if err := os.MkdirAll(filepath.Dir(contentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("return true\n")
	if err := os.WriteFile(contentPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	manifest := ContentManifest{Version: "1", Files: map[string]string{"script/npc.lua": hex.EncodeToString(hash[:])}}
	manifestPath := filepath.Join(root, "manifest.json")
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadContentManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyContent(root, loaded); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contentPath, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyContent(root, loaded); err == nil {
		t.Fatal("expected content hash mismatch")
	}
}
