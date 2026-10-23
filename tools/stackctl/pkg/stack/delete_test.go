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

package stack

import (
	"context"
	"fmt"
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

func TestDeleteAtResourceGroup(t *testing.T) {
	for _, tc := range []struct {
		name        string
		stacks      fakeStacks
		want        Result
		wantErr     string
		wantDeletes int
	}{
		{
			name:        "existing stack is deleted",
			want:        ResultDeleted,
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
			name:    "lookup failure is returned and nothing is deleted",
			stacks:  fakeStacks{getStatus: http.StatusForbidden, getErrorCode: "AuthorizationFailed"},
			wantErr: "failed to get deployment stack",
		},
		{
			name:        "stack deleted by someone else in the meantime is a no-op",
			stacks:      fakeStacks{deleteStatus: http.StatusNotFound, deleteError: "DeploymentStackResourceNotFound"},
			want:        ResultNotFound,
			wantDeletes: 1,
		},
		{
			name:        "rejected delete is returned",
			stacks:      fakeStacks{deleteStatus: http.StatusForbidden, deleteError: "AuthorizationFailed"},
			wantErr:     "failed to start deleting deployment stack",
			wantDeletes: 1,
		},
		{
			name:        "failed delete is returned",
			stacks:      fakeStacks{pollError: "DeploymentStackDeleteFailed"},
			wantErr:     "failed to delete deployment stack",
			wantDeletes: 1,
		},
	} {
		for _, action := range []ActionOnUnmanage{ActionDetach, ActionDelete} {
			t.Run(fmt.Sprintf("%s/%s", action, tc.name), func(t *testing.T) {
				stacks := tc.stacks
				got, err := DeleteAtResourceGroup(context.Background(), stacks.client(t), testResourceGroup, testStack, action)
				if tc.wantErr != "" {
					require.ErrorContains(t, err, tc.wantErr)
				} else {
					require.NoError(t, err)
					require.Equal(t, tc.want, got)
				}
				require.Equal(t, tc.wantDeletes, stacks.deleteCalls)
				if tc.wantDeletes > 0 {
					want, err := deleteOptions(action)
					require.NoError(t, err)
					require.Equal(t, want, stacks.deleteOptions, "delete must use the requested action on unmanage")
				}
			})
		}
	}
}

func TestDeleteAtResourceGroupRejectsUnknownActionBeforeCallingAzure(t *testing.T) {
	stacks := fakeStacks{}
	_, err := DeleteAtResourceGroup(context.Background(), stacks.client(t), testResourceGroup, testStack, "")
	require.ErrorContains(t, err, "invalid action on unmanage")
	require.Zero(t, stacks.deleteCalls)
}

func TestDeleteOptions(t *testing.T) {
	detach, err := deleteOptions(ActionDetach)
	require.NoError(t, err)
	require.Equal(t, armdeploymentstacks.UnmanageActionResourceModeDetach, *detach.UnmanageActionResources)
	require.Equal(t, armdeploymentstacks.UnmanageActionResourceGroupModeDetach, *detach.UnmanageActionResourceGroups)
	require.Equal(t, armdeploymentstacks.UnmanageActionManagementGroupModeDetach, *detach.UnmanageActionManagementGroups)
	require.Nil(t, detach.BypassStackOutOfSyncError)

	del, err := deleteOptions(ActionDelete)
	require.NoError(t, err)
	require.Equal(t, armdeploymentstacks.UnmanageActionResourceModeDelete, *del.UnmanageActionResources)
	require.Equal(t, armdeploymentstacks.UnmanageActionResourceGroupModeDelete, *del.UnmanageActionResourceGroups)
	require.Equal(t, armdeploymentstacks.UnmanageActionManagementGroupModeDelete, *del.UnmanageActionManagementGroups)
	require.Nil(t, del.BypassStackOutOfSyncError)

	_, err = deleteOptions("deleteAll")
	require.ErrorContains(t, err, "invalid action on unmanage")
}

func TestParseActionOnUnmanage(t *testing.T) {
	for _, in := range []string{"detach", "delete"} {
		got, err := ParseActionOnUnmanage(in)
		require.NoError(t, err)
		require.Equal(t, ActionOnUnmanage(in), got)
	}
	for _, in := range []string{"", "Detach", "detachAll", "deleteResources"} {
		_, err := ParseActionOnUnmanage(in)
		require.ErrorContains(t, err, "invalid action on unmanage", "input %q", in)
	}
}
