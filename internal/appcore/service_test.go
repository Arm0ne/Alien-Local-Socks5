package appcore

import (
	"testing"

	"realityconverter/internal/converter"
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
