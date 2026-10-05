package sync

import (
	"testing"
)

func TestNewCommand(t *testing.T) {
	cmd, err := NewCommand()
	if err != nil {
		t.Fatalf("unexpected error creating command: %v", err)
	}
	if cmd.Use != "sync" {
		t.Errorf("expected Use 'sync', got %q", cmd.Use)
	}

	// Verify all expected flags exist
	expectedFlags := []string{
		"target-acr",
		"source-registry",
		"repository",
		"digest",
		"copy-from",
		"image-file-path",
		"image-tar-file",
		"image-metadata-file",
		"image-tar-sas",
		"image-metadata-sas",
		"pull-secret-kv",
		"pull-secret",
		"dry-run",
	}
	for _, flag := range expectedFlags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}
