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

package grafana

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/grafana-tools/sdk"
	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/testutil"
)

// httpGoldenFixture is the CompareWithFixture name for TestHTTPGolden
// (testdata/<subdir>/zz_fixture_<TestName>.json).
const httpGoldenFixture = "zz_fixture_TestHTTPGolden.json"

// goldenExchange is one Grafana round-trip stored for replay. The long-lived
// test serves ResponseBody and asserts the current client still makes the same
// request (method/path/query/headers/body).
type goldenExchange struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	Path           string          `json:"path"`
	Query          string          `json:"query,omitempty"`
	Authorization  string          `json:"authorization"`
	Accept         string          `json:"accept,omitempty"`
	ContentType    string          `json:"contentType,omitempty"`
	UserAgent      string          `json:"userAgent,omitempty"`
	RequestBody    json.RawMessage `json:"requestBody,omitempty"`
	ResponseStatus int             `json:"responseStatus"`
	ResponseBody   json.RawMessage `json:"responseBody,omitempty"`
}

func TestHTTPGolden(t *testing.T) {
	ops := httpGoldenOps(t)
	goldenPath := filepath.Join("testdata", "http", httpGoldenFixture)
	want := loadGolden(t, goldenPath)
	require.Len(t, want, len(ops), "golden operations must match httpGoldenOps(); run UPDATE=1 to refresh")

	wantByName := map[string]goldenExchange{}
	for _, rec := range want {
		wantByName[rec.Name] = rec
	}

	var mu sync.Mutex
	var got []goldenExchange
	var current string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)

		mu.Lock()
		name := current
		rec := goldenExchange{
			Name:           name,
			Method:         request.Method,
			Path:           request.URL.Path,
			Query:          request.URL.RawQuery,
			Authorization:  request.Header.Get("Authorization"),
			Accept:         request.Header.Get("Accept"),
			ContentType:    request.Header.Get("Content-Type"),
			UserAgent:      request.Header.Get("User-Agent"),
			ResponseStatus: http.StatusOK,
		}
		if len(bytes.TrimSpace(body)) > 0 {
			rec.RequestBody = json.RawMessage(append([]byte(nil), body...))
		}
		seed, ok := wantByName[name]
		require.True(t, ok, "golden missing %s", name)
		require.NotEmpty(t, seed.ResponseBody, "golden missing responseBody for %s", name)
		if seed.ResponseStatus != 0 {
			rec.ResponseStatus = seed.ResponseStatus
		}
		rec.ResponseBody = append([]byte(nil), seed.ResponseBody...)
		got = append(got, rec)
		status := rec.ResponseStatus
		payload := rec.ResponseBody
		mu.Unlock()

		writeJSON(writer, status, string(payload))
	}))
	defer server.Close()

	client := newGoldenTestClient(t, server.URL, "test-token")
	for _, o := range ops {
		mu.Lock()
		current = o.name
		mu.Unlock()
		o.call(t, client)
	}

	mu.Lock()
	recorded := got
	mu.Unlock()
	require.Len(t, recorded, len(ops))

	payload, err := json.MarshalIndent(recorded, "", "  ")
	require.NoError(t, err)
	testutil.CompareWithFixture(t, append(payload, '\n'), testutil.WithExtension(".json"), testutil.WithSubDir("http"))
}

type httpGoldenOp struct {
	name string
	call func(*testing.T, *Client)
}

func httpGoldenOps(t *testing.T) []httpGoldenOp {
	ctx := t.Context()
	return []httpGoldenOp{
		{
			name: "ListDataSources",
			call: func(t *testing.T, c *Client) {
				_, err := c.ListDataSources(ctx)
				require.NoError(t, err)
			},
		},
		{
			name: "DeleteDataSource",
			call: func(t *testing.T, c *Client) {
				require.NoError(t, c.DeleteDataSource(ctx, "Prometheus"))
			},
		},
		{
			name: "ListFolders",
			call: func(t *testing.T, c *Client) {
				_, err := c.ListFolders(ctx)
				require.NoError(t, err)
			},
		},
		{
			name: "ListDashboards",
			call: func(t *testing.T, c *Client) {
				_, err := c.ListDashboards(ctx)
				require.NoError(t, err)
			},
		},
		{
			name: "SearchFolders",
			call: func(t *testing.T, c *Client) {
				_, err := c.SearchFolders(ctx)
				require.NoError(t, err)
			},
		},
		{
			name: "CreateFolder",
			call: func(t *testing.T, c *Client) {
				_, err := c.CreateFolder(ctx, "Scratchpad")
				require.NoError(t, err)
			},
		},
		{
			name: "GetRawDashboardByUID",
			call: func(t *testing.T, c *Client) {
				_, _, err := c.GetRawDashboardByUID(ctx, "test-dashboard")
				require.NoError(t, err)
			},
		},
		{
			name: "SetRawDashboard",
			call: func(t *testing.T, c *Client) {
				require.NoError(t, c.SetRawDashboard(ctx, []byte(dashboardJSON), 12, true))
			},
		},
		{
			name: "DeleteDashboardByUID",
			call: func(t *testing.T, c *Client) {
				require.NoError(t, c.DeleteDashboardByUID(ctx, "test-dashboard"))
			},
		},
		{
			name: "GetFolderPermissions",
			call: func(t *testing.T, c *Client) {
				_, err := c.GetFolderPermissions(ctx, "folder-uid")
				require.NoError(t, err)
			},
		},
		{
			name: "UpdateFolderPermissions",
			call: func(t *testing.T, c *Client) {
				require.NoError(t, c.UpdateFolderPermissions(ctx, "folder-uid",
					sdk.FolderPermission{Role: "Viewer", Permission: sdk.PermissionEdit},
					sdk.FolderPermission{Role: "Editor", Permission: sdk.PermissionEdit},
					sdk.FolderPermission{Role: "Admin", Permission: sdk.PermissionAdmin},
				))
			},
		},
		{
			name: "DeleteFolderByUID",
			call: func(t *testing.T, c *Client) {
				require.NoError(t, c.DeleteFolderByUID(ctx, "folder-uid"))
			},
		},
	}
}

func newGoldenTestClient(t *testing.T, serverURL, token string) *Client {
	t.Helper()
	sdkClient, err := sdk.NewClient(serverURL, token, http.DefaultClient)
	require.NoError(t, err)
	return &Client{grafanaClient: sdkClient}
}

func loadGolden(t *testing.T, path string) []goldenExchange {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "missing %s; run UPDATE=1 after seeding response bodies", path)
	var out []goldenExchange
	require.NoError(t, json.Unmarshal(raw, &out), path)
	return out
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
