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

package discoverbootstrap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"github.com/stretchr/testify/require"
)

type testCredential struct{}

func (testCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "test", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCommandDiscoveryOutput(t *testing.T) {
	for _, tt := range []struct {
		name, body, want string
		status           int
		jsonVersions     bool
		nextBody         string
		nextStatus       int
	}{
		{name: "absent exact ordered CSV", body: `{"value":[]}`, want: "asm-1-29,asm-1-28"},
		{name: "absent native JSON config", body: `{"value":[]}`, want: "asm-1-29,asm-1-28", jsonVersions: true},
		{name: "existing failed", body: `{"value":[{"name":"TARGET","properties":{"provisioningState":"Failed"}}]}`},
		{name: "existing canary", body: `{"value":[{"name":"target","properties":{"serviceMeshProfile":{"mode":"Istio","istio":{"revisions":["asm-1-28","asm-1-29"]}}}}]}`},
		{name: "permission error", body: `{"error":{"code":"AuthorizationFailed","message":"denied"}}`, status: 403},
		{name: "absent after pagination", body: `{"value":[],"nextLink":"https://management.azure.com/subscriptions/sub/resourceGroups/rg/providers/Microsoft.ContainerService/managedClusters?api-version=2025-07-01&$skiptoken=next"}`, nextBody: `{"value":[]}`, want: "asm-1-29,asm-1-28"},
		{name: "existing on second page", body: `{"value":[],"nextLink":"https://management.azure.com/subscriptions/sub/resourceGroups/rg/providers/Microsoft.ContainerService/managedClusters?api-version=2025-07-01&$skiptoken=next"}`, nextBody: `{"value":[{"name":"target"}]}`},
		{name: "later page error publishes nothing", body: `{"value":[],"nextLink":"https://management.azure.com/subscriptions/sub/resourceGroups/rg/providers/Microsoft.ContainerService/managedClusters?api-version=2025-07-01&$skiptoken=next"}`, nextBody: `{"error":{"code":"InternalServerError","message":"failed"}}`, nextStatus: 500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			cmd, err := newCommand(func(subscription string) (*armcontainerservice.ManagedClustersClient, error) {
				require.Equal(t, "sub", subscription)
				return armcontainerservice.NewManagedClustersClient(subscription, testCredential{}, &arm.ClientOptions{ClientOptions: policy.ClientOptions{
					Retry: policy.RetryOptions{MaxRetries: -1},
					Transport: testTransport(func(req *http.Request) (*http.Response, error) {
						requests++
						require.Equal(t, http.MethodGet, req.Method)
						require.Equal(t, "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.ContainerService/managedClusters", req.URL.Path)
						status := tt.status
						body := tt.body
						if requests > 1 {
							require.Equal(t, 2, requests)
							require.Equal(t, "next", req.URL.Query().Get("$skiptoken"))
							body = tt.nextBody
							status = tt.nextStatus
						}
						if status == 0 {
							status = http.StatusOK
						}
						return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
					}),
				}})
			})
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "outputs.json")
			args := []string{"--subscription-id", "sub", "--resource-group", "rg", "--cluster-name", "target", "--output-file", path}
			if tt.jsonVersions {
				args = append(args, "--versions-json", `["asm-1-29","asm-1-28"]`)
			} else {
				args = append(args, "--versions", "asm-1-29,asm-1-28")
			}
			cmd.SetArgs(args)
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			err = cmd.Execute()
			wantRequests := 1
			if tt.nextBody != "" {
				wantRequests = 2
			}
			require.Equal(t, wantRequests, requests)
			if tt.status != 0 || tt.nextStatus != 0 {
				require.Error(t, err)
				require.Empty(t, stdout.String(), "failed listing must never publish absent-cluster output")
				_, statErr := os.Stat(path)
				require.ErrorIs(t, statErr, os.ErrNotExist)
				return
			}
			require.NoError(t, err)
			want := fmt.Sprintf("{\"istioVersions\":%q}\n", tt.want)
			require.Equal(t, want, stdout.String())
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, want, string(data))
		})
	}
}

func TestCommandRejectsInvalidConfigurationBeforeCredentials(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--versions", ""},
		{"--versions", "asm-1-29,"},
		{"--versions", "asm-1-29, asm-1-28"},
		{"--versions-json", `[]`},
		{"--versions-json", `null`},
		{"--versions-json", `"asm-1-29"`},
		{"--versions-json", `["asm-1-29",42]`},
		{"--versions-json", `["asm-1-29"]`, "--versions", "asm-1-29"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd, err := newCommand(func(string) (*armcontainerservice.ManagedClustersClient, error) {
				t.Fatal("invalid input must not construct credentials or SDK client")
				return nil, nil
			})
			require.NoError(t, err)
			cmd.SetArgs(append([]string{"--subscription-id", "sub", "--resource-group", "rg", "--cluster-name", "target"}, args...))
			require.Error(t, cmd.Execute())
		})
	}
}

func TestCommandClientFailurePublishesNothing(t *testing.T) {
	cmd, err := newCommand(func(string) (*armcontainerservice.ManagedClustersClient, error) {
		return nil, fmt.Errorf("credential unavailable")
	})
	require.NoError(t, err)
	cmd.SetArgs([]string{"--subscription-id", "sub", "--resource-group", "rg", "--cluster-name", "target", "--versions", "asm-1-29"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	require.ErrorContains(t, cmd.Execute(), "credential unavailable")
	require.Empty(t, stdout.String())
}
