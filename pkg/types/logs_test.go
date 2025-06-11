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

func TestNewLogsStep(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected LogsStep
		err      bool
	}{
		{
			name: "TestNewRPLogsAccount_ValidInput",
			input: `
name: rplog
action: RPLogsAccount`,
			expected: *NewRPLogsAccountStep("rplog"),
		},
		{
			name: "TestNewClusterLogsAccount_ValidInput",
			input: `
name: cllog
action: ClusterLogsAccount`,
			expected: *NewClusterLogsAccountStep("cllog"),
		},
		{
			name: "TestNewRPLogsAccount_Complete",
			input: `
name: test
action: RPLogsAccount
dependsOn: ["foo-bar"]
subscriptionId:
  name: subscriptionId
  value: subscriptionId
namespace:
  name: namespace
  value: namespace
certsan:
  name: certsan
  value: certsan
certdescription:
  name: certdescription
  value: certdescription
configVersion:
  name: configVersion
  value: configVersion
events:
  akskubesystem: foo
`,
			expected: *NewRPLogsAccountStep("test").
				WithSubscriptionId(Variable{Name: "subscriptionId", Value: "subscriptionId"}).
				WithNamespace(Variable{Name: "namespace", Value: "namespace"}).
				WithCertDescription(Variable{Name: "certdescription", Value: "certdescription"}).
				WithCertSAN(Variable{Name: "certsan", Value: "certsan"}).
				WithConfigVersion(Variable{Name: "configVersion", Value: "configVersion"}).
				WithDependsOn("foo-bar").
				WithEvents("foo"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output LogsStep

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
