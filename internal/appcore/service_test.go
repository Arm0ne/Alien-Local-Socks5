package appcore

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"realityconverter/internal/converter"
	"realityconverter/internal/profile"
	"realityconverter/internal/session"
)

func TestSourceWithoutNode(t *testing.T) {
	nodes := []converter.Node{
		{RawLink: "vless://node-1"},
		{RawLink: "vless://node-2"},
		{RawLink: "vless://node-3"},
	}

	got, err := sourceWithoutNode(nodes, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := "vless://node-1\r\nvless://node-3"; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}

	got, err = sourceWithoutNode(nodes, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := "vless://node-2\r\nvless://node-3"; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}

	if _, err := sourceWithoutNode(nodes, 3); err == nil {
		t.Fatal("expected an invalid index error")
	}
}

func TestSourceWithoutLastNodeIsEmpty(t *testing.T) {
	source, err := sourceWithoutNode([]converter.Node{{RawLink: "vless://only"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if source != "" {
		t.Fatalf("source = %q, want empty", source)
	}
}

func TestPreservePortsWhenSubscriptionOrderChanges(t *testing.T) {
	previous := converter.ParseResult{
		Nodes: []converter.Node{{Server: "a.example"}, {Server: "b.example"}},
		Mappings: []converter.Mapping{
			{Index: 1, ListenPort: 21001},
			{Index: 2, ListenPort: 21004},
		},
	}
	next := converter.ParseResult{
		Nodes: []converter.Node{{Server: "b.example"}, {Server: "c.example"}, {Server: "a.example"}},
		Mappings: []converter.Mapping{
			{Index: 1, ListenPort: 21001},
			{Index: 2, ListenPort: 21002},
			{Index: 3, ListenPort: 21003},
		},
	}
	got := preservePorts(previous, next, 21001)
	ports := []int{got.Mappings[0].ListenPort, got.Mappings[1].ListenPort, got.Mappings[2].ListenPort}
	want := []int{21004, 21005, 21001}
	for index := range want {
		if ports[index] != want[index] {
			t.Fatalf("ports = %v, want %v", ports, want)
		}
		if got.Mappings[index].Index != index+1 {
			t.Fatalf("mapping %d index = %d", index, got.Mappings[index].Index)
		}
	}
}

func TestPreservePortsAfterNodeRemoval(t *testing.T) {
	previous := converter.ParseResult{
		Nodes: []converter.Node{{Server: "a.example"}, {Server: "b.example"}, {Server: "c.example"}},
		Mappings: []converter.Mapping{
			{Index: 1, ListenPort: 21001},
			{Index: 2, ListenPort: 21002},
			{Index: 3, ListenPort: 21003},
		},
	}
	next := converter.ParseResult{
		Nodes: []converter.Node{{Server: "a.example"}, {Server: "c.example"}},
		Mappings: []converter.Mapping{
			{Index: 1, ListenPort: 21001},
			{Index: 2, ListenPort: 21002},
		},
	}
	got := preservePorts(previous, next, 21001)
	if got.Mappings[0].ListenPort != 21001 || got.Mappings[1].ListenPort != 21003 {
		t.Fatalf("ports = %d, %d, want 21001, 21003", got.Mappings[0].ListenPort, got.Mappings[1].ListenPort)
	}
}

func TestSingleNodeResultKeepsSelectedMapping(t *testing.T) {
	result := converter.ParseResult{
		Nodes: []converter.Node{{Server: "a.example"}, {Server: "b.example"}},
		Mappings: []converter.Mapping{
			{Index: 1, InboundTag: "ads-01", OutboundTag: "reality-01", ListenPort: 21001},
			{Index: 2, InboundTag: "ads-02", OutboundTag: "reality-02", ListenPort: 21008},
		},
	}
	selected, err := singleNodeResult(result, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Nodes) != 1 || selected.Nodes[0].Server != "b.example" {
		t.Fatalf("unexpected selected node: %#v", selected.Nodes)
	}
	if len(selected.Mappings) != 1 || selected.Mappings[0].ListenPort != 21008 || selected.Mappings[0].OutboundTag != "reality-02" {
		t.Fatalf("unexpected selected mapping: %#v", selected.Mappings)
	}
	if _, err := singleNodeResult(result, 2); err == nil {
		t.Fatal("expected an invalid index error")
	}
}

func TestProbeNodeStopsTemporaryXrayAfterCheckFailure(t *testing.T) {
	xrayPath := os.Getenv("XRAY_TEST_EXE")
	if xrayPath == "" {
		t.Skip("XRAY_TEST_EXE is not set")
	}
	t.Setenv("XRAY_EXE", xrayPath)

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	key := make([]byte, 32)
	for index := range key {
		key[index] = byte(index + 1)
	}
	result := converter.ParseResult{
		Nodes: []converter.Node{{
			Server:      "127.0.0.1",
			ServerPort:  1,
			UUID:        "00000001-1234-4abc-8def-000000000001",
			Network:     "tcp",
			Encryption:  "none",
			Security:    "reality",
			PublicKey:   base64.RawURLEncoding.EncodeToString(key),
			Fingerprint: "chrome",
			ServerName:  "example.com",
			ShortID:     "1234",
			SpiderX:     "/",
			Flow:        "xtls-rprx-vision",
		}},
		Mappings: []converter.Mapping{{Index: 1, InboundTag: "ads-01", OutboundTag: "reality-01", ListenPort: port}},
	}
	directory := t.TempDir()
	service := &Service{
		dataDir: directory,
		store:   profile.Store{Path: filepath.Join(directory, "profile.dat")},
		session: session.NewManager(),
		result:  result,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := service.ProbeNode(ctx, 0); err == nil {
		t.Fatal("expected the unreachable Reality node check to fail")
	}
	if service.IsRunning() {
		t.Fatal("temporary probe changed the main session state")
	}
	released, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)))
	if err != nil {
		t.Fatalf("temporary probe did not release port %d: %v", port, err)
	}
	_ = released.Close()
	if matches, _ := filepath.Glob(filepath.Join(directory, "probe", ".running-config-*.json")); len(matches) != 0 {
		t.Fatalf("temporary probe config was not removed: %v", matches)
	}
}
