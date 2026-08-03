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

package manage

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultReconcileOptions(t *testing.T) {
	opts := DefaultReconcileOptions()

	if opts.SKU != "Standard" {
		t.Fatalf("expected default SKU 'Standard', got %q", opts.SKU)
	}
	if opts.ZoneRedundancy != "Disabled" {
		t.Fatalf("expected default ZoneRedundancy 'Disabled', got %q", opts.ZoneRedundancy)
	}
	if opts.PublicNetworkAccess != "Enabled" {
		t.Fatalf("expected default PublicNetworkAccess 'Enabled', got %q", opts.PublicNetworkAccess)
	}
}

func TestValidatePublicNetworkAccess(t *testing.T) {
	for _, tc := range []struct {
		name               string
		publicNetworkAccess string
		wantErrSub         string
	}{
		{
			name:               "Enabled is valid",
			publicNetworkAccess: "Enabled",
		},
		{
			name:               "Disabled is valid",
			publicNetworkAccess: "Disabled",
		},
		{
			name:               "empty string is rejected",
			publicNetworkAccess: "",
			wantErrSub:         "--public-network-access must be",
		},
		{
			name:               "invalid value is rejected",
			publicNetworkAccess: "Invalid",
			wantErrSub:         "--public-network-access must be",
		},
		{
			name:               "lowercase is rejected",
			publicNetworkAccess: "enabled",
			wantErrSub:         "--public-network-access must be",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := DefaultReconcileOptions()
			opts.Location = "eastus"
			opts.GrafanaName = "test-grafana"
			opts.SubscriptionID = "00000000-0000-0000-0000-000000000000"
			opts.ResourceGroup = "test-rg"
			opts.PublicNetworkAccess = tc.publicNetworkAccess

			_, err := opts.Validate(context.Background())
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
		})
	}
}
