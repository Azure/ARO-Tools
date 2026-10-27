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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	configtypes "github.com/Azure/ARO-Tools/config/types"
)

func TestHelmStepValidateEv2CompatibilityBudget(t *testing.T) {
	for _, tt := range []struct {
		name           string
		extraNamespace string
		wantErr        bool
	}{
		{
			name: "53 distinct names with duplicates across files pass",
		},
		{
			name:           "54th name in namespace exceeds budget despite equal values",
			extraNamespace: "extra: {{ .extra }}\n",
			wantErr:        true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configuration := configtypes.Configuration{"extra": "same-value"}
			var values strings.Builder
			var namespace strings.Builder
			// 52 configuration names plus one literal input name leave no headroom.
			for i := range 52 {
				name := fmt.Sprintf("p%d", i)
				configuration[name] = "same-value"
				fmt.Fprintf(&values, "%s: {{ .%s }}\n%sAgain: {{ .%s }}\n", name, name, name, name)
				fmt.Fprintf(&namespace, "%s: {{ .%s }}\n", name, name)
			}
			values.WriteString("input: __clientId__\nalias: __p0__\n")
			namespace.WriteString("input: __clientId__\n" + tt.extraNamespace)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "values.yaml"), []byte(values.String()), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "namespace.yaml"), []byte(namespace.String()), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "other-namespace.yaml"), []byte("alias: __p0__\n"), 0o644))
			step := &HelmStep{
				StepMeta:       StepMeta{Name: "backend"},
				ValuesFile:     "values.yaml",
				NamespaceFiles: []string{"namespace.yaml", "other-namespace.yaml"},
				InputVariables: map[string]Input{
					"p0":       {StepDependency: StepDependency{ResourceGroup: "infra", Step: "identity"}, Name: "clientId"},
					"clientId": {StepDependency: StepDependency{ResourceGroup: "infra", Step: "identity"}, Name: "clientId"},
				},
			}

			err := step.ValidateEv2Compatibility(dir, configuration)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "backend")
				require.Contains(t, err.Error(), "54 templated parameters")
				require.Contains(t, err.Error(), "budget of 53")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestHelmStepValidateEv2CompatibilityMissingFile(t *testing.T) {
	step := &HelmStep{ValuesFile: "missing.yaml"}
	err := step.ValidateEv2Compatibility(t.TempDir(), nil)
	require.ErrorIs(t, err, os.ErrNotExist)
}
