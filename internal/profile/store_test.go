package profile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRoundTripIsEncrypted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.dat")
	store := Store{Path: path}
	source := "vless://sensitive-node-link"
	if err := store.Save(source, 21001); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), source) {
		t.Fatal("profile contains plaintext source")
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SourceTXT != source || loaded.StartPort != 21001 {
		t.Fatalf("unexpected profile: %#v", loaded)
	}
}

func TestStoreMissing(t *testing.T) {
	_, err := (Store{Path: filepath.Join(t.TempDir(), "missing.dat")}).Load()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func TestStoreDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.dat")
	store := Store{Path: path}
	if err := store.Save("vless://sensitive-node-link", 21001); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("load after delete error = %v", err)
	}
	if err := store.Delete(); err != nil {
		t.Fatalf("second delete error = %v", err)
	}
}
