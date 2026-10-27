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

package istio

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
)

type bootstrapCredential struct{}

func (bootstrapCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "test", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

type bootstrapTransport func(*http.Request) (*http.Response, error)

func (f bootstrapTransport) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDiscoverBootstrap(t *testing.T) {
	const next = `,"nextLink":"https://management.azure.com/subscriptions/sub/resourceGroups/rg/providers/Microsoft.ContainerService/managedClusters?api-version=2025-07-01&$skiptoken=next"`
	for _, tt := range []struct {
		name   string
		pages  []string
		status int
		want   []string
		err    bool
	}{
		{name: "absent", pages: []string{`{"value":[]}`}, want: []string{"asm-1-29", "asm-1-28"}},
		{name: "absent after all pages", pages: []string{`{"value":[{"name":"other"}]` + next + `}`, `{"value":[]}`}, want: []string{"asm-1-29", "asm-1-28"}},
		{name: "existing case insensitive", pages: []string{`{"value":[{"name":"TaRgEt"}]}`}, want: []string{}},
		{name: "existing failed", pages: []string{`{"value":[{"name":"target","properties":{"provisioningState":"Failed"}}]}`}, want: []string{}},
		{name: "existing without mesh", pages: []string{`{"value":[{"name":"target","properties":{"provisioningState":"Succeeded"}}]}`}, want: []string{}},
		{name: "existing canary preserves both revisions", pages: []string{`{"value":[{"name":"target","properties":{"serviceMeshProfile":{"mode":"Istio","istio":{"revisions":["asm-1-28","asm-1-29"]}}}}]}`}, want: []string{}},
		{name: "existing on later page", pages: []string{`{"value":[{"name":"other"}]` + next + `}`, `{"value":[{"name":"target"}]}`}, want: []string{}},
		{name: "first page forbidden", pages: []string{`{"error":{"code":"AuthorizationFailed","message":"denied"}}`}, status: 403, err: true},
		{name: "first page not found is not absence", pages: []string{`{"error":{"code":"ResourceGroupNotFound","message":"missing group"}}`}, status: 404, err: true},
		{name: "later page failure is not absence", pages: []string{`{"value":[]` + next + `}`, `{"error":{"code":"InternalServerError","message":"failed"}}`}, status: 500, err: true},
		{name: "must finish listing even after match", pages: []string{`{"value":[{"name":"target"}]` + next + `}`, `{"error":{"code":"InternalServerError","message":"failed"}}`}, status: 500, err: true},
		{name: "malformed page", pages: []string{`not JSON`}, err: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			client, err := armcontainerservice.NewManagedClustersClient("sub", bootstrapCredential{}, &arm.ClientOptions{ClientOptions: policy.ClientOptions{
				Retry: policy.RetryOptions{MaxRetries: -1},
				Transport: bootstrapTransport(func(req *http.Request) (*http.Response, error) {
					require.Equal(t, http.MethodGet, req.Method)
					require.Equal(t, "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.ContainerService/managedClusters", req.URL.Path, "discovery must never GET the target cluster")
					require.Less(t, calls, len(tt.pages))
					status := http.StatusOK
					if tt.status != 0 && calls == len(tt.pages)-1 {
						status = tt.status
					}
					body := tt.pages[calls]
					calls++
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
				}),
			}})
			require.NoError(t, err)
			configured := []string{"asm-1-29", "asm-1-28"}
			result, err := DiscoverBootstrap(context.Background(), client, "rg", "target", configured)
			require.Equal(t, len(tt.pages), calls)
			if tt.err {
				require.Error(t, err)
				require.Nil(t, result)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tt.want, result)
			if len(result) > 0 {
				result[0] = "changed"
				require.Equal(t, "asm-1-29", configured[0], "discovery must not alias configured revisions")
			}
		})
	}
}

func TestDiscoverBootstrapInvalidConfiguration(t *testing.T) {
	client, err := armcontainerservice.NewManagedClustersClient("sub", bootstrapCredential{}, &arm.ClientOptions{ClientOptions: policy.ClientOptions{
		Transport: bootstrapTransport(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid configuration must not contact ARM")
			return nil, nil
		}),
	}})
	require.NoError(t, err)
	for _, tt := range []struct {
		rg, cluster string
		versions    []string
	}{
		{rg: "rg", cluster: "target"},
		{rg: "rg", cluster: "target", versions: []string{""}},
		{rg: "rg", cluster: "target", versions: []string{"asm-1-29", "invalid"}},
		{rg: "", cluster: "target", versions: []string{"asm-1-29"}},
		{rg: "rg", cluster: " ", versions: []string{"asm-1-29"}},
	} {
		result, err := DiscoverBootstrap(context.Background(), client, tt.rg, tt.cluster, tt.versions)
		require.Error(t, err)
		require.Nil(t, result)
	}
	_, err = DiscoverBootstrap(context.Background(), nil, "rg", "target", []string{"asm-1-29"})
	require.Error(t, err)
}

func TestDiscoverBootstrapCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, err := armcontainerservice.NewManagedClustersClient("sub", bootstrapCredential{}, nil)
	require.NoError(t, err)
	result, err := DiscoverBootstrap(ctx, client, "rg", "target", []string{"asm-1-29"})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
}
