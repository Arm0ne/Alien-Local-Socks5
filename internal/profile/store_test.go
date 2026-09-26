package profile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestStoreSubscriptionRoundTripPreservesMetadataAndPorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.dat")
	store := Store{Path: path}
	expiresAt := time.Unix(1893456000, 0).Local()
	fetchedAt := time.Unix(1893450000, 0).Local()
	source := "vless://subscription-node"
	if err := store.SaveSubscription(source, "https://example.com/sub?token=secret", &expiresAt, &fetchedAt, "订阅到期时间格式无效", 21001, []int{21001, 21004}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SourceTXT != source || loaded.SourceKind != SourceKindSubscription {
		t.Fatalf("unexpected source: %#v", loaded)
	}
	if loaded.SubscriptionURL != "https://example.com/sub?token=secret" || loaded.SubscriptionExpiresAt != expiresAt.Unix() || loaded.SubscriptionFetchedAt != fetchedAt.Unix() {
		t.Fatalf("unexpected subscription metadata: %#v", loaded)
	}
	if loaded.SubscriptionMetadataNote != "订阅到期时间格式无效" || len(loaded.ListenPorts) != 2 || loaded.ListenPorts[1] != 21004 {
		t.Fatalf("unexpected ports or notice: %#v", loaded)
	}
}

func TestStoreFileRoundTripPreservesEncryptedPathAndPorts(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "profile.dat")
	store := Store{Path: profilePath}
	source := "vless://file-node"
	sourcePath := filepath.Join(t.TempDir(), "private-nodes.txt")
	if err := store.SaveFile(source, sourcePath, 21001, []int{21001, 21004}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), source) || strings.Contains(string(raw), sourcePath) {
		t.Fatal("profile exposes plaintext source or file path")
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SourceKind != SourceKindFile || loaded.SourcePath != sourcePath {
		t.Fatalf("unexpected file source: %#v", loaded)
	}
	if len(loaded.ListenPorts) != 2 || loaded.ListenPorts[0] != 21001 || loaded.ListenPorts[1] != 21004 {
		t.Fatalf("unexpected file ports: %#v", loaded.ListenPorts)
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
