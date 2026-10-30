package imagemirror

import (
	"testing"
)

func TestNewCommand(t *testing.T) {
	cmd, err := NewCommand()
	if err != nil {
		t.Fatalf("unexpected error creating command: %v", err)
	}
	if cmd.Use != "image-mirror" {
		t.Errorf("expected Use 'image-mirror', got %q", cmd.Use)
	}

	// Verify the "sync" subcommand is registered
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "sync" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'sync' subcommand not found")
	}
}
