package xrayruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndReuseVerifiedRuntime(t *testing.T) {
	executable := []byte("test xray executable")
	hash := sha256.Sum256(executable)
	expected := hex.EncodeToString(hash[:])
	directory := t.TempDir()
	path, err := Install(directory, executable, expected)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(executable) {
		t.Fatalf("installed data = %q", content)
	}
	secondPath, err := Install(directory, executable, expected)
	if err != nil || secondPath != path {
		t.Fatalf("reuse failed: %s, %v", secondPath, err)
	}
	if filepath.Base(path) != "xray.exe" {
		t.Fatalf("path = %s", path)
	}
}

func TestInstallRejectsHashMismatch(t *testing.T) {
	if _, err := Install(t.TempDir(), []byte("bad"), "0000"); err == nil {
		t.Fatal("expected hash mismatch")
	}
}
