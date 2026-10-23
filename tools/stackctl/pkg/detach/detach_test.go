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

package detach

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeploymentstacks"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeploymentstacks/fake"
)

const (
	testResourceGroup = "global-shared-resources"
	testStack         = "Global.singleton.acrs"
)

// fakeStacks configures what the fake ARM server returns and records the delete calls it receives.
type fakeStacks struct {
	getStatus    int
	getErrorCode string
	deleteStatus int
	deleteError  string
	pollError    string

	deleteCalls   int
	deleteOptions *armdeploymentstacks.ClientBeginDeleteAtResourceGroupOptions
}

func (f *fakeStacks) client(t *testing.T) *armdeploymentstacks.Client {
	t.Helper()
	srv := fake.Server{
		GetAtResourceGroup: func(_ context.Context, resourceGroupName, deploymentStackName string, _ *armdeploymentstacks.ClientGetAtResourceGroupOptions) (resp azfake.Responder[armdeploymentstacks.ClientGetAtResourceGroupResponse], errResp azfake.ErrorResponder) {
			require.Equal(t, testResourceGroup, resourceGroupName)
			require.Equal(t, testStack, deploymentStackName)
			if f.getErrorCode != "" {
				errResp.SetResponseError(f.getStatus, f.getErrorCode)
				return
			}
			resp.SetResponse(http.StatusOK, armdeploymentstacks.ClientGetAtResourceGroupResponse{
				DeploymentStack: armdeploymentstacks.DeploymentStack{
					Name: to.Ptr(testStack),
					Properties: &armdeploymentstacks.DeploymentStackProperties{
						Resources: []*armdeploymentstacks.ManagedResourceReference{
							{ID: to.Ptr("/subscriptions/sub/resourceGroups/global-shared-resources/providers/Microsoft.ContainerRegistry/registries/arohcpsvcint")},
							{ID: to.Ptr("/subscriptions/sub/resourceGroups/global-shared-resources/providers/Microsoft.ContainerRegistry/registries/arohcpocpint")},
						},
					},
				},
			}, nil)
			return
		},
		BeginDeleteAtResourceGroup: func(_ context.Context, resourceGroupName, deploymentStackName string, options *armdeploymentstacks.ClientBeginDeleteAtResourceGroupOptions) (resp azfake.PollerResponder[armdeploymentstacks.ClientDeleteAtResourceGroupResponse], errResp azfake.ErrorResponder) {
			require.Equal(t, testResourceGroup, resourceGroupName)
			require.Equal(t, testStack, deploymentStackName)
			f.deleteCalls++
			f.deleteOptions = options
			if f.deleteError != "" {
				errResp.SetResponseError(f.deleteStatus, f.deleteError)
				return
			}
			resp.AddNonTerminalResponse(http.StatusAccepted, nil)
			if f.pollError != "" {
				resp.SetTerminalError(http.StatusConflict, f.pollError)
				return
			}
			resp.SetTerminalResponse(http.StatusOK, armdeploymentstacks.ClientDeleteAtResourceGroupResponse{}, nil)
			return
		},
	}
	client, err := armdeploymentstacks.NewClient("sub", &azfake.TokenCredential{}, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{Transport: fake.NewServerTransport(&srv)},
	})
	require.NoError(t, err)
	return client
}

func TestAtResourceGroup(t *testing.T) {
	for _, tc := range []struct {
		name        string
		stacks      fakeStacks
		want        Result
		wantErr     string
		wantDeletes int
	}{
		{
			name:        "existing stack is removed and its resources are detached",
			want:        ResultDetached,
			wantDeletes: 1,
		},
		{
			name:   "missing stack is a no-op",
			stacks: fakeStacks{getStatus: http.StatusNotFound, getErrorCode: "DeploymentStackResourceNotFound"},
			want:   ResultNotFound,
		},
		{
			name:   "missing resource group is a no-op",
			stacks: fakeStacks{getStatus: http.StatusNotFound, getErrorCode: "ResourceGroupNotFound"},
			want:   ResultNotFound,
		},
		{
			name:    "lookup failure is returned and nothing is removed",
			stacks:  fakeStacks{getStatus: http.StatusForbidden, getErrorCode: "AuthorizationFailed"},
			wantErr: "failed to get deployment stack",
		},
		{
			name:        "stack removed by someone else in the meantime is a no-op",
			stacks:      fakeStacks{deleteStatus: http.StatusNotFound, deleteError: "DeploymentStackResourceNotFound"},
			want:        ResultNotFound,
			wantDeletes: 1,
		},
		{
			name:        "rejected removal is returned",
			stacks:      fakeStacks{deleteStatus: http.StatusForbidden, deleteError: "AuthorizationFailed"},
			wantErr:     "failed to start removing deployment stack",
			wantDeletes: 1,
		},
		{
			name:        "failed removal is returned",
			stacks:      fakeStacks{pollError: "DeploymentStackDeleteFailed"},
			wantErr:     "failed to remove deployment stack",
			wantDeletes: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AtResourceGroup(context.Background(), tc.stacks.client(t), testResourceGroup, testStack)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
			require.Equal(t, tc.wantDeletes, tc.stacks.deleteCalls)
			if tc.wantDeletes > 0 {
				require.Equal(t, deleteOptions(), tc.stacks.deleteOptions, "removal must always detach")
			}
		})
	}
}

// TestDeleteOptionsAlwaysDetach guards the one property this package exists for: removing a stack never deletes
// the resources, resource groups or management groups it manages.
func TestDeleteOptionsAlwaysDetach(t *testing.T) {
	opts := deleteOptions()
	require.Equal(t, armdeploymentstacks.UnmanageActionResourceModeDetach, *opts.UnmanageActionResources)
	require.Equal(t, armdeploymentstacks.UnmanageActionResourceGroupModeDetach, *opts.UnmanageActionResourceGroups)
	require.Equal(t, armdeploymentstacks.UnmanageActionManagementGroupModeDetach, *opts.UnmanageActionManagementGroups)
	require.Nil(t, opts.BypassStackOutOfSyncError)
}
