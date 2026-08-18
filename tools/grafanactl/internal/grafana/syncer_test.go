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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const dashboardJSON = `{
  "id": null,
  "uid": "test-dashboard",
  "title": "Test Dashboard",
  "version": 7,
  "templating": {
    "list": [
      {
        "name": "datasource",
        "query": "prometheus",
        "regex": "/managed-prometheus/",
        "type": "datasource"
      }
    ]
  },
  "panels": [
    {
      "id": 1,
      "title": "Status",
      "type": "table",
      "transformations": [
        {
          "id": "renameByRegex",
          "options": {
            "regex": "^Value$",
            "renamePattern": "Time in Progress (Minutes)"
          }
        }
      ],
      "pluginSpecificField": {
        "preserve": true
      }
    }
  ]
}`

func TestReadDashboardFilePreservesUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.json")
	require.NoError(t, os.WriteFile(path, []byte(dashboardJSON), 0o600))

	dashboard, err := readDashboardFile(path)
	require.NoError(t, err)
	require.Equal(t, "test-dashboard", dashboard.meta.UID)
	require.Equal(t, "Test Dashboard", dashboard.meta.Title)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(dashboard.raw, &raw))
	panel := raw["panels"].([]interface{})[0].(map[string]interface{})
	require.Contains(t, panel, "transformations")
	require.Contains(t, panel, "pluginSpecificField")
}

func TestReadDashboardFileUnwrapsDashboard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.json")
	wrapped := `{"dashboard":` + dashboardJSON + `,"meta":{"folderId":12}}`
	require.NoError(t, os.WriteFile(path, []byte(wrapped), 0o600))

	dashboard, err := readDashboardFile(path)
	require.NoError(t, err)
	require.Equal(t, "test-dashboard", dashboard.meta.UID)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(dashboard.raw, &raw))
	require.NotContains(t, raw, "dashboard")
	require.Contains(t, raw, "panels")
}

func TestReadDashboardFileReturnsWrappedParseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"dashboard":"invalid"}`), 0o600))

	_, err := readDashboardFile(path)
	require.ErrorContains(t, err, "failed to parse wrapped dashboard JSON")
}

func TestReadDashboardFileFallsBackForRawDashboardField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.json")
	raw := `{"dashboard":{},"uid":"test-dashboard","title":"Test Dashboard"}`
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	dashboard, err := readDashboardFile(path)
	require.NoError(t, err)
	require.Equal(t, "test-dashboard", dashboard.meta.UID)
	require.JSONEq(t, raw, string(dashboard.raw))
}

func TestAreDashboardsEqual(t *testing.T) {
	remote := []byte(dashboardJSON)
	local := []byte(dashboardJSON)
	require.True(t, areDashboardsEqual(remote, local))
	require.False(t, areDashboardsEqual([]byte("null"), local))

	var changedID map[string]interface{}
	require.NoError(t, json.Unmarshal(remote, &changedID))
	changedID["id"] = 42
	changedID["version"] = 99
	remote, err := json.Marshal(changedID)
	require.NoError(t, err)
	require.True(t, areDashboardsEqual(remote, local))

	var changedTransformation map[string]interface{}
	require.NoError(t, json.Unmarshal(local, &changedTransformation))
	panel := changedTransformation["panels"].([]interface{})[0].(map[string]interface{})
	transform := panel["transformations"].([]interface{})[0].(map[string]interface{})
	transform["id"] = "organize"
	local, err = json.Marshal(changedTransformation)
	require.NoError(t, err)
	require.False(t, areDashboardsEqual(remote, local))
}

func TestNormalizeDashboardClearsGrafanaManagedFields(t *testing.T) {
	normalized, err := normalizeDashboard([]byte(dashboardJSON))
	require.NoError(t, err)

	var dashboard map[string]interface{}
	require.NoError(t, json.Unmarshal(normalized, &dashboard))
	require.Equal(t, float64(0), dashboard["id"])
	require.Equal(t, float64(0), dashboard["version"])

	panel := dashboard["panels"].([]interface{})[0].(map[string]interface{})
	require.Contains(t, panel, "transformations")
	require.Contains(t, panel, "pluginSpecificField")
}

func TestNormalizeDashboardRejectsTrailingData(t *testing.T) {
	_, err := normalizeDashboard([]byte(dashboardJSON + ` garbage`))
	require.ErrorContains(t, err, "dashboard contains trailing data")

	_, err = normalizeDashboard([]byte(dashboardJSON + ` {}`))
	require.ErrorContains(t, err, "dashboard contains multiple JSON values")
}
