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

	"github.com/Azure/ARO-Tools/internal/testutil"
)

func TestResolveImageMirrorStep(t *testing.T) {
	tests := []struct {
		name       string
		input      ImageMirrorStep
		scriptFile string
	}{
		{
			name: "image-mirror-step",
			input: ImageMirrorStep{
				StepMeta: StepMeta{
					Name:   "image-mirror-step",
					Action: "ImageMirror",
				},
				TargetACR:          Value{Value: "myacr.azurecr.io"},
				SourceRegistry:     Value{Value: "docker.io"},
				Repository:         Value{Value: "nginx"},
				Digest:             Value{Value: "sha256:123456"},
				PullSecretKeyVault: Value{Value: "my-keyvault"},
				PullSecretName:     Value{Value: "my-pull-secret"},
				ShellIdentity:      Value{Value: "my-identity"},
			},
			scriptFile: "/path/to/script.sh",
		},
		{
			name: "image-mirror-step-with-deps",
			input: ImageMirrorStep{
				StepMeta: StepMeta{
					Name:      "image-mirror-step",
					Action:    "ImageMirror",
					DependsOn: []StepDependency{{ResourceGroup: "whatever", Step: "previous-step"}},
				},
				TargetACR:          Value{Value: "myacr.azurecr.io"},
				SourceRegistry:     Value{Value: "docker.io"},
				Repository:         Value{Value: "nginx"},
				Digest:             Value{Value: "sha256:123456"},
				PullSecretKeyVault: Value{Value: "my-keyvault"},
				PullSecretName:     Value{Value: "my-pull-secret"},
				ShellIdentity:      Value{Value: "my-identity"},
			},
			scriptFile: "/path/to/script.sh",
		},
		{
			name: "image-mirror-step-with-oci-layout",
			input: ImageMirrorStep{
				StepMeta: StepMeta{
					Name:   "image-mirror-step",
					Action: "ImageMirror",
				},
				TargetACR:      Value{Value: "myacr.azurecr.io"},
				SourceRegistry: Value{Value: "docker.io"},
				Repository:     Value{Value: "nginx"},
				Digest:         Value{Value: "sha256:123456"},
				CopyFrom:       "oci-layout",
				OCILayoutPath:  Value{Value: "/path/to/oci-layout"},
				ShellIdentity:  Value{Value: "my-identity"},
			},
			scriptFile: "/path/to/script.sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveImageMirrorStep(tt.input, tt.scriptFile)
			if err != nil {
				t.Fatalf("ResolveImageMirrorStep() error = %v", err)
			}

			testutil.CompareWithFixture(t, result, testutil.WithExtension(".yaml"))
		})
	}
}
