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
	"gopkg.in/yaml.v3"
)

func TestNewDNSStep(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected DNSStep
		err      bool
	}{
		{
			name: "TestNewDNSStep_ValidInput",
			input: `
name: test-zone
action: DelegateChildZone`,
			expected: *NewDNSStep("test-zone"),
		},
		{
			name: "TestAllOptions",
			input: `
name: test-zone
action: DelegateChildZone
dependsOn: ["foo-bar"]
childZone:
  name: childZone
  value: child
parentZone:
  name: parentZone
  value: parent`,
			expected: *NewDNSStep("test-zone").
				WithChild(Variable{Name: "childZone", Value: "child"}).
				WithParent(Variable{Name: "parentZone", Value: "parent"}).
				WithDependsOn("foo-bar"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output DNSStep

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
