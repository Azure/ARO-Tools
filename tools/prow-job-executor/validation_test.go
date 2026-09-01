// Copyright 2026 Microsoft Corporation
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

package prowjobexecutor

import "testing"

func TestValidateHTTPURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{name: "https URL", rawURL: "https://gangway.example.com/v1/executions", wantErr: false},
		{name: "http URL", rawURL: "http://gangway.example.com/v1/executions", wantErr: false},
		{name: "empty string", rawURL: "", wantErr: true},
		{name: "missing scheme", rawURL: "gangway.example.com/v1/executions", wantErr: true},
		{name: "unsupported scheme", rawURL: "ftp://gangway.example.com/v1/executions", wantErr: true},
		{name: "unparseable", rawURL: "://not a url", wantErr: true},
		{name: "scheme with no host", rawURL: "https://", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHTTPURL("test-url", tc.rawURL)
			if tc.wantErr && err == nil {
				t.Fatalf("validateHTTPURL(%q) = nil, want an error", tc.rawURL)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateHTTPURL(%q) = %v, want no error", tc.rawURL, err)
			}
		})
	}
}
