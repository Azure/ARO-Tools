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

package base

import (
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
)

func TestNormalizeEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name       string
		input      string
		want       string
		wantErrSub string
	}{
		{
			name:  "hostname is prefixed with https",
			input: "management.usgovcloudapi.net",
			want:  "https://management.usgovcloudapi.net",
		},
		{
			name:  "full https URL is returned unchanged",
			input: "https://management.usgovcloudapi.net",
			want:  "https://management.usgovcloudapi.net",
		},
		{
			name:  "https URL with trailing slash is preserved",
			input: "https://login.microsoftonline.us/",
			want:  "https://login.microsoftonline.us/",
		},
		{
			name:  "leading and trailing whitespace is trimmed",
			input: "  management.azure.com  ",
			want:  "https://management.azure.com",
		},
		{
			name:       "empty string returns an error",
			input:      "",
			wantErrSub: "endpoint cannot be empty",
		},
		{
			name:       "whitespace-only string returns an error",
			input:      "   ",
			wantErrSub: "endpoint cannot be empty",
		},
		{
			name:       "missing host is rejected",
			input:      "https://",
			wantErrSub: "host cannot be empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeEndpoint(tc.input)
			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (result %q)", tc.wantErrSub, got)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErrSub, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveCloudConfig(t *testing.T) {
	for _, tc := range []struct {
		name         string
		armEndpoint  string
		aadAuthority string
		wantPublic   bool
		wantARM      string
		wantAAD      string
		wantErrSub   string
	}{
		{
			name:       "both empty defaults to public cloud",
			wantPublic: true,
		},
		{
			name:         "only ARM endpoint set returns an error",
			armEndpoint:  "management.usgovcloudapi.net",
			aadAuthority: "",
			wantErrSub:   "must both be set",
		},
		{
			name:         "only AAD authority set returns an error",
			armEndpoint:  "",
			aadAuthority: "login.microsoftonline.us",
			wantErrSub:   "must both be set",
		},
		{
			name:         "both set as hostnames builds Gov cloud config",
			armEndpoint:  "management.usgovcloudapi.net",
			aadAuthority: "login.microsoftonline.us",
			wantARM:      "https://management.usgovcloudapi.net",
			wantAAD:      "https://login.microsoftonline.us",
		},
		{
			name:         "both set as full URLs builds Gov cloud config",
			armEndpoint:  "https://management.usgovcloudapi.net",
			aadAuthority: "https://login.microsoftonline.us/",
			wantARM:      "https://management.usgovcloudapi.net",
			wantAAD:      "https://login.microsoftonline.us/",
		},
		{
			name:         "empty ARM endpoint after trimming surfaces a wrapped error",
			armEndpoint:  "   ",
			aadAuthority: "login.microsoftonline.us",
			wantErrSub:   "invalid --arm-endpoint",
		},
		{
			name:         "empty AAD authority after trimming surfaces a wrapped error",
			armEndpoint:  "management.usgovcloudapi.net",
			aadAuthority: "   ",
			wantErrSub:   "invalid --aad-authority",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveCloudConfig(tc.armEndpoint, tc.aadAuthority)
			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrSub)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErrSub, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantPublic {
				if got.ActiveDirectoryAuthorityHost != cloud.AzurePublic.ActiveDirectoryAuthorityHost {
					t.Fatalf("expected public cloud authority, got %q", got.ActiveDirectoryAuthorityHost)
				}
				return
			}
			if got.ActiveDirectoryAuthorityHost != tc.wantAAD {
				t.Fatalf("AAD authority: got %q, want %q", got.ActiveDirectoryAuthorityHost, tc.wantAAD)
			}
			armSvc, ok := got.Services[cloud.ResourceManager]
			if !ok {
				t.Fatal("expected ResourceManager service in cloud configuration")
			}
			if armSvc.Endpoint != tc.wantARM {
				t.Fatalf("ARM endpoint: got %q, want %q", armSvc.Endpoint, tc.wantARM)
			}
			if armSvc.Audience != tc.wantARM {
				t.Fatalf("ARM audience: got %q, want %q", armSvc.Audience, tc.wantARM)
			}
		})
	}
}
