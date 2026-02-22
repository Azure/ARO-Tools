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
		name string
		vars types.Configuration
		path string
		want any
		err  string
	}{
		{
			name: "simple",
			vars: types.Configuration{
				"key": "value",
			},
			path: "key",
			want: "value",
		},
		{
			name: "nested",
			vars: types.Configuration{
				"parent": map[string]any{
					"key": "value",
				},
			},
			path: "parent.key",
			want: "value",
		},
		{
			name: "missing",
			vars: types.Configuration{
				"parent": map[string]any{
					"key": "value",
				},
			},
			path: "parent.key2",
			err:  "configuration[parent]: key key2 not found",
		},
		{
			name: "invalid type",
			vars: types.Configuration{
				"parent": map[string]any{
					"key": "value",
				},
			},
			path: "parent.key.nested",
			err:  "configuration[parent][key]: expected nested map, found string; cannot index with nested",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.vars.GetByPath(tt.path)
			if got != tt.want {
				t.Errorf("got = %v, want %v", got, tt.want)
			}
			if err != nil && err.Error() != tt.err || err == nil && tt.err != "" {
				t.Errorf("expected error %s, got %s", tt.err, err)
			}
		})
	}
}
