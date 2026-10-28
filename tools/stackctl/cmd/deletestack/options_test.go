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

package deletestack

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/tools/cmdutils"
)

func validRawOptions() *RawOptions {
	return &RawOptions{
		CloudOptions:     &cmdutils.RawOptions{Cloud: "public"},
		SubscriptionID:   "00000000-0000-0000-0000-000000000000",
		ResourceGroup:    "global-shared-resources",
		Name:             "Global.singleton.acrs",
		ActionOnUnmanage: "detach",
		Timeout:          time.Minute,
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(o *RawOptions)
		wantErr string
	}{
		{name: "valid detach", mutate: func(o *RawOptions) {}},
		{name: "valid delete", mutate: func(o *RawOptions) { o.ActionOnUnmanage = "delete" }},
		{name: "missing action on unmanage", mutate: func(o *RawOptions) { o.ActionOnUnmanage = "" }, wantErr: "--action-on-unmanage is required"},
		{name: "invalid action on unmanage", mutate: func(o *RawOptions) { o.ActionOnUnmanage = "detachAll" }, wantErr: "invalid --action-on-unmanage"},
		{name: "missing cloud", mutate: func(o *RawOptions) { o.CloudOptions.Cloud = "" }, wantErr: "--cloud"},
		{name: "unknown cloud", mutate: func(o *RawOptions) { o.CloudOptions.Cloud = "mars" }, wantErr: "invalid cloud"},
		{name: "missing subscription", mutate: func(o *RawOptions) { o.SubscriptionID = "" }, wantErr: "--subscription is required"},
		{name: "missing resource group", mutate: func(o *RawOptions) { o.ResourceGroup = "" }, wantErr: "--resource-group is required"},
		{name: "missing name", mutate: func(o *RawOptions) { o.Name = "" }, wantErr: "--name is required"},
		{name: "invalid name", mutate: func(o *RawOptions) { o.Name = "Global/singleton acrs" }, wantErr: "invalid --name"},
		{name: "non-positive timeout", mutate: func(o *RawOptions) { o.Timeout = 0 }, wantErr: "--timeout must be positive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := validRawOptions()
			tc.mutate(opts)
			_, err := opts.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestNewCommandRequiresFlags(t *testing.T) {
	cmd, err := NewCommand()
	require.NoError(t, err)
	cmd.SetArgs([]string{})
	require.ErrorContains(t, cmd.Execute(), "required flag(s)")
}
