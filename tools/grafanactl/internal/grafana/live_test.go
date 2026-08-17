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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/tools/grafanactl/internal/azure"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

const (
	liveGrafanaEnv       = "GRAFANACTL_LIVE"
	liveGrafanaUpdateEnv = "GRAFANACTL_LIVE_UPDATE"
	// Default target is the shared RH-tenant Grafana used to seed HTTP goldens.
	// Override with GRAFANACTL_SUBSCRIPTION / RESOURCE_GROUP / GRAFANA_NAME.
	liveDefaultSubscription  = "1d3378d3-5a3f-4712-85a1-2485495dfc4b"
	liveDefaultResourceGroup = "global"
	liveDefaultGrafanaName   = "arohcp-dev"
	liveFolderUID            = "ffpxq3s5iwg74b"
	liveDashboardUID         = "access-cluster-slo"
	// liveSeedLimit is passed as Grafana's limit query param (or used to slice
	// APIs that have no limit) so seeded response bodies stay small.
	liveSeedLimit = 3
)

// TestSeedHTTPGolden hits arohcp-dev and writes real response bodies into
// testdata/http/golden.json. Skipped unless GRAFANACTL_LIVE=1. Read-only: lists
// and one dashboard GET, with limit= so the fixture stays small. The long-lived
// CI test is TestHTTPGolden, which replays those bodies without live Grafana.
func TestSeedHTTPGolden(t *testing.T) {
	if os.Getenv(liveGrafanaEnv) != "1" {
		t.Skip("set GRAFANACTL_LIVE=1 to seed HTTP golden response bodies from arohcp-dev")
	}

	ctx := t.Context()
	cred, err := azidentity.NewAzureCLICredential(nil)
	require.NoError(t, err, "Azure CLI credentials")

	subscription := envOr(liveDefaultSubscription, "GRAFANACTL_SUBSCRIPTION")
	resourceGroup := envOr(liveDefaultResourceGroup, "GRAFANACTL_RESOURCE_GROUP")
	grafanaName := envOr(liveDefaultGrafanaName, "GRAFANACTL_GRAFANA_NAME")

	managed, err := azure.NewManagedGrafanaClient(subscription, cred, nil)
	require.NoError(t, err)
	client, err := NewClient(ctx, cred, managed, subscription, resourceGroup, grafanaName)
	require.NoError(t, err)

	datasources, err := seedGET(ctx, client, "datasources", nil)
	require.NoError(t, err, "ListDataSources")
	datasources = firstNJSONArray(t, datasources, liveSeedLimit)

	folders, err := seedGET(ctx, client, "folders", url.Values{"limit": {fmt.Sprint(liveSeedLimit)}})
	require.NoError(t, err, "ListFolders")

	dashboards, err := seedGET(ctx, client, "search", url.Values{
		"type":  {searchTypeDashboard},
		"limit": {fmt.Sprint(liveSeedLimit)},
	})
	require.NoError(t, err, "ListDashboards")

	searchFolders, err := seedGET(ctx, client, "search", url.Values{
		"type":  {searchTypeFolder},
		"limit": {fmt.Sprint(liveSeedLimit)},
	})
	require.NoError(t, err, "SearchFolders")

	permissions, err := seedGET(ctx, client, "folders/"+liveFolderUID+"/permissions", nil)
	require.NoError(t, err, "GetFolderPermissions")
	permissions = redactDumpPII(dumpJSONBody(permissions))

	dashboard, err := seedGET(ctx, client, "dashboards/uid/"+liveDashboardUID, nil)
	require.NoError(t, err, "GetRawDashboardByUID")

	if os.Getenv(liveGrafanaUpdateEnv) != "1" {
		require.NotEmpty(t, datasources)
		require.NotEmpty(t, folders)
		require.NotEmpty(t, dashboards)
		require.NotEmpty(t, searchFolders)
		require.NotEmpty(t, permissions)
		require.NotEmpty(t, dashboard)
		return
	}

	goldenPath := filepath.Join("testdata", "http", httpGoldenFixture)
	golden := loadGolden(t, goldenPath)
	bodies := map[string]json.RawMessage{
		"ListDataSources":       datasources,
		"ListFolders":           folders,
		"ListDashboards":        dashboards,
		"SearchFolders":         searchFolders,
		"GetFolderPermissions":  permissions,
		"GetRawDashboardByUID":  dashboard,
	}
	for i, rec := range golden {
		if body, ok := bodies[rec.Name]; ok {
			golden[i].ResponseBody = body
		}
	}
	payload, err := json.MarshalIndent(golden, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(goldenPath, append(payload, '\n'), 0o644))
}

func seedGET(ctx context.Context, c *Client, relPath string, query url.Values) (json.RawMessage, error) {
	u := c.apiBase.JoinPath(splitPath(relPath)...)
	if query != nil {
		u.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: status %d", u.Path, resp.StatusCode)
	}
	return json.RawMessage(raw), nil
}

func splitPath(rel string) []string {
	var parts []string
	for _, p := range strings.Split(rel, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func firstNJSONArray(t *testing.T, raw json.RawMessage, n int) json.RawMessage {
	t.Helper()
	var items []json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &items))
	require.NotEmpty(t, items)
	if len(items) > n {
		items = items[:n]
	}
	out, err := json.Marshal(items)
	require.NoError(t, err)
	return out
}

func envOr(def, key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
