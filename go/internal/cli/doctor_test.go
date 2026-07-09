package cli

import (
	"testing"

	"github.com/santhosh2011/sdev/internal/state"
)

func TestDuplicateOffsetsFindsRepeats(t *testing.T) {
	l := &state.Ledger{Tasks: map[string]state.Task{
		"a": {Offset: 10}, "b": {Offset: 10}, "c": {Offset: 20},
	}}

	got := duplicateOffsets(l)

	if len(got) != 1 || got[0] != 10 {
		t.Fatalf("duplicateOffsets = %v, want [10]", got)
	}
}

func TestDuplicateOffsetsNoneWhenDistinct(t *testing.T) {
	l := &state.Ledger{Tasks: map[string]state.Task{
		"a": {Offset: 10}, "b": {Offset: 20},
	}}

	if got := duplicateOffsets(l); len(got) != 0 {
		t.Fatalf("duplicateOffsets = %v, want []", got)
	}
}
