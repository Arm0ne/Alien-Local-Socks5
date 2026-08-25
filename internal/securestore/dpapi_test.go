package securestore

import (
	"bytes"
	"testing"
)

func TestDPAPIRoundTrip(t *testing.T) {
	plainText := []byte("vless://sensitive-test-value")
	cipherText, err := Protect(plainText)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(cipherText, plainText) {
		t.Fatal("encrypted output contains plaintext")
	}
	decrypted, err := Unprotect(cipherText)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, plainText) {
		t.Fatalf("decrypted data = %q", decrypted)
	}
}

func TestDPAPIRejectsEmptyInput(t *testing.T) {
	if _, err := Protect(nil); err == nil {
		t.Fatal("Protect should reject empty input")
	}
	if _, err := Unprotect(nil); err == nil {
		t.Fatal("Unprotect should reject empty input")
	}
}
