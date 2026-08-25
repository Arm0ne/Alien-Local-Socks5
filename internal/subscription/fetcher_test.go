package subscription

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testVLESS = "vless://00000001-1234-4abc-8def-000000000001@server.example.com:443?type=tcp&encryption=none&security=reality&pbk=AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA&fp=chrome&sni=example.com&sid=1234&spx=%2F"

func TestFetchPlainTextAndExpiry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("User-Agent") != "AlienLocalSocks5/1.3" {
			t.Fatalf("unexpected user agent: %q", request.Header.Get("User-Agent"))
		}
		writer.Header().Set("subscription-userinfo", "upload=1; download=2; total=3; expire=1893456000")
		_, _ = writer.Write([]byte(testVLESS + "\r\n"))
	}))
	defer server.Close()

	document, err := (Fetcher{}).Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if document.SourceTXT != testVLESS {
		t.Fatalf("source = %q", document.SourceTXT)
	}
	if document.ExpiresAt == nil || document.ExpiresAt.Unix() != 1893456000 {
		t.Fatalf("expiry = %v", document.ExpiresAt)
	}
	if document.FetchedAt.IsZero() {
		t.Fatal("missing fetched time")
	}
}

func TestFetchBase64Subscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		encoded := base64.RawURLEncoding.EncodeToString([]byte(testVLESS + "\n"))
		_, _ = writer.Write([]byte(encoded))
	}))
	defer server.Close()

	document, err := (Fetcher{}).Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if document.SourceTXT != testVLESS {
		t.Fatalf("source = %q", document.SourceTXT)
	}
}

func TestFetchExpiryNoticeAndHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("subscription-userinfo", "expire=not-a-timestamp")
		_, _ = writer.Write([]byte(testVLESS))
	}))
	document, err := (Fetcher{}).Fetch(context.Background(), server.URL)
	server.Close()
	if err != nil {
		t.Fatal(err)
	}
	if document.MetadataNotice == "" || document.ExpiresAt != nil {
		t.Fatalf("unexpected metadata: %#v", document)
	}

	statusServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer statusServer.Close()
	if _, err := (Fetcher{}).Fetch(context.Background(), statusServer.URL); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("unexpected HTTP error: %v", err)
	}
}

func TestDecodeSourceRejectsUnsupportedContent(t *testing.T) {
	if _, err := decodeSource([]byte("not a VLESS subscription")); err == nil {
		t.Fatal("expected unsupported content error")
	}
	if _, err := decodeSource([]byte("")); err == nil {
		t.Fatal("expected empty content error")
	}
}

func TestParseExpiryWithoutHeader(t *testing.T) {
	expiry, notice := parseExpiry("")
	if expiry != nil || notice != "" {
		t.Fatalf("expiry=%v notice=%q", expiry, notice)
	}
	if _, notice := parseExpiry("expire=0"); notice == "" {
		t.Fatal("expected invalid expiry notice")
	}
	if _, notice := parseExpiry("total=1"); notice != "" {
		t.Fatalf("unexpected notice: %q", notice)
	}
}
