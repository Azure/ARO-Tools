// Copyright 2025 Microsoft Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package types

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"sigs.k8s.io/yaml"
)

func TestNewShellStep(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ShellStep
		err      bool
	}{
		{
			name: "TestNewShell_ValidInput",
			input: `
name: test
action: Shell
command: command`,
			expected: *NewShellStep("test", "command"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output ShellStep

			err := yaml.Unmarshal([]byte(tt.input), &output)
			if (err != nil) != tt.err {
				t.Fatalf("UnmarshalYAML() error = %v, expectedError %v", err, tt.err)
			}

			if diff := cmp.Diff(tt.expected, output, nil); diff != "" {
				t.Fatalf("UnmarshalYAML() mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}
