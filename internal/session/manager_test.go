package session

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"realityconverter/internal/converter"
)

func TestStartRejectsOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	result := testResult(port)
	configJSON, err := converter.BuildConfig(result)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	err = manager.Start(context.Background(), "missing-xray.exe", t.TempDir(), configJSON, result)
	var conflict PortConflictError
	if !errors.As(err, &conflict) || len(conflict.Ports) != 1 || conflict.Ports[0] != port {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartAndStopRealXray(t *testing.T) {
	xrayPath := os.Getenv("XRAY_TEST_EXE")
	if xrayPath == "" {
		t.Skip("XRAY_TEST_EXE is not set")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	result := testResult(port)
	configJSON, err := converter.BuildConfig(result)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	manager := NewManager()
	if err := manager.Start(context.Background(), xrayPath, directory, configJSON, result); err != nil {
		t.Fatal(err)
	}
	if !manager.IsRunning() {
		t.Fatal("manager should be running")
	}
	connection, err := net.DialTimeout("tcp4", listenerAddress(port), time.Second)
	if err != nil {
		t.Fatalf("SOCKS port is not listening: %v", err)
	}
	_ = connection.Close()
	if matches, _ := filepath.Glob(filepath.Join(directory, ".running-config-*.json")); len(matches) != 0 {
		t.Fatalf("runtime config should be removed after startup: %v", matches)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.IsRunning() {
		t.Fatal("manager should be stopped")
	}
	if connection, err := net.DialTimeout("tcp4", listenerAddress(port), 200*time.Millisecond); err == nil {
		_ = connection.Close()
		t.Fatal("SOCKS port still accepts connections")
	}
}

func testResult(port int) converter.ParseResult {
	key := make([]byte, 32)
	for index := range key {
		key[index] = byte(index + 1)
	}
	node := converter.Node{
		SourceLine:  1,
		Server:      "server.example.com",
		ServerPort:  443,
		UUID:        "00000001-1234-4abc-8def-000000000001",
		Network:     "tcp",
		Encryption:  "none",
		Security:    "reality",
		PublicKey:   base64.RawURLEncoding.EncodeToString(key),
		Fingerprint: "chrome",
		ServerName:  "sni.example.com",
		ShortID:     "0000000000000001",
		SpiderX:     "/",
		Flow:        "xtls-rprx-vision",
	}
	mapping := converter.Mapping{Index: 1, InboundTag: "ads-01", OutboundTag: "reality-01", ListenPort: port}
	return converter.ParseResult{Nodes: []converter.Node{node}, Mappings: []converter.Mapping{mapping}}
}

func listenerAddress(port int) string {
	return net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port))
}
