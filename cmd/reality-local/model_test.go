package main

import "testing"

func TestPortTableModelResetPublishesChangedRange(t *testing.T) {
	model := &portTableModel{}
	resetCount := 0
	changedFrom, changedTo := -1, -1
	model.RowsReset().Attach(func() {
		resetCount++
	})
	model.RowsChanged().Attach(func(from, to int) {
		changedFrom, changedTo = from, to
	})

	model.reset([]portRow{{Status: statusNormal}, {Status: statusStopped}})

	if resetCount != 1 {
		t.Fatalf("RowsReset published %d times, want 1", resetCount)
	}
	if changedFrom != 0 || changedTo != 1 {
		t.Fatalf("RowsChanged range = %d..%d, want 0..1", changedFrom, changedTo)
	}
}

func TestPortTableModelEmptyResetDoesNotPublishInvalidRange(t *testing.T) {
	model := &portTableModel{}
	changed := false
	model.RowsChanged().Attach(func(_, _ int) {
		changed = true
	})

	model.reset(nil)

	if changed {
		t.Fatal("RowsChanged published for an empty model")
	}
}
