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

package config

import (
	"testing"

	"github.com/Azure/ARO-Tools/pkg/config/types"
)

func TestGetByPath(t *testing.T) {
	tests := []struct {
		name  string
		vars  types.Configuration
		path  string
		want  any
		found bool
	}{
		{
			name: "simple",
			vars: types.Configuration{
				"key": "value",
			},
			path:  "key",
			want:  "value",
			found: true,
		},
		{
			name: "nested",
			vars: types.Configuration{
				"key": types.Configuration{
					"key": "value",
				},
			},
			path:  "key.key",
			want:  "value",
			found: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := tt.vars.GetByPath(tt.path)
			if got != tt.want {
				t.Errorf("Configuration.GetByPath() got = %v, want %v", got, tt.want)
			}
			if found != tt.found {
				t.Errorf("Configuration.GetByPath() found = %v, want %v", found, tt.found)
			}
		})
	}
}
