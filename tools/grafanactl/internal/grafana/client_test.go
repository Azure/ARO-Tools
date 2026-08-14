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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana-tools/sdk"
	"github.com/stretchr/testify/require"
)

func TestSetRawDashboardPreservesUnknownFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/dashboards/db", request.URL.Path)

		var payload struct {
			Dashboard map[string]interface{} `json:"dashboard"`
			FolderID  int                    `json:"folderId"`
			Overwrite bool                   `json:"overwrite"`
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
		require.Equal(t, 12, payload.FolderID)
		require.True(t, payload.Overwrite)

		dashboard := payload.Dashboard
		require.Equal(t, float64(0), dashboard["id"])
		panel := dashboard["panels"].([]interface{})[0].(map[string]interface{})
		require.Contains(t, panel, "transformations")
		require.Contains(t, panel, "pluginSpecificField")

		writer.Header().Set("Content-Type", "application/json")
		_, err := writer.Write([]byte(`{"status":"success","uid":"test-dashboard"}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	sdkClient, err := sdk.NewClient(server.URL, "", server.Client())
	require.NoError(t, err)
	client := &Client{grafanaClient: sdkClient}

	require.NoError(t, client.SetRawDashboard(context.Background(), []byte(dashboardJSON), 12, true))
}

func TestGetRawDashboardPreservesUnknownFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/dashboards/uid/test-dashboard", request.URL.Path)

		writer.Header().Set("Content-Type", "application/json")
		_, err := writer.Write([]byte(`{"dashboard":` + dashboardJSON + `,"meta":{"folderId":12}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	sdkClient, err := sdk.NewClient(server.URL, "", server.Client())
	require.NoError(t, err)
	client := &Client{grafanaClient: sdkClient}

	raw, properties, err := client.GetRawDashboardByUID(context.Background(), "test-dashboard")
	require.NoError(t, err)
	require.Equal(t, 12, properties.FolderID)

	var dashboard map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &dashboard))
	panel := dashboard["panels"].([]interface{})[0].(map[string]interface{})
	require.Contains(t, panel, "transformations")
	require.Contains(t, panel, "pluginSpecificField")
}
