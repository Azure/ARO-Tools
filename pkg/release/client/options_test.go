package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr/testr"

	"github.com/Azure/ARO-Tools/internal/testutil"
)

func TestParseComponentsYAML(t *testing.T) {
	tests := []struct {
		name         string
		inputPath    string
		wantErr      bool
		wantWarnings bool
	}{
		{
			name:         "OK",
			inputPath:    filepath.Join("testdata", "inputs", "config_ok.yaml"),
			wantErr:      false,
			wantWarnings: false,
		},
		{
			name:         "MissingComponents",
			inputPath:    filepath.Join("testdata", "inputs", "config_missing_components.yaml"),
			wantErr:      false,
			wantWarnings: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testr.New(t)

			content, err := os.ReadFile(tt.inputPath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", tt.inputPath, err)
			}

			got, gotWarnings, gotErr := parseComponentsYAML(logger, content)
			if gotErr != nil {
				if !tt.wantErr {
					t.Fatalf("parseComponentsYAML() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("parseComponentsYAML() succeeded unexpectedly")
			}
			if tt.wantWarnings && len(gotWarnings) == 0 {
				t.Fatalf("parseComponentsYAML() expected warnings, got none")
			}
			if !tt.wantWarnings && len(gotWarnings) != 0 {
				t.Fatalf("parseComponentsYAML() returned warnings unexpectedly: %v", gotWarnings)
			}

			testutil.CompareWithFixture(t, got, testutil.WithExtension(".yaml"))
		})
	}
}
