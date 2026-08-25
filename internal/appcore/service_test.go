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
