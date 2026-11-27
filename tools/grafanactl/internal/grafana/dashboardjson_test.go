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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareDashboardForUploadAppliesOverrides(t *testing.T) {
	raw := []byte(`{"uid":"original-uid","title":"Original Title","id":41,"version":9}`)

	prepared, err := PrepareDashboardForUpload(raw, "pr-123-original-uid", "PR-123 Original Title")
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(prepared, &got))
	require.Equal(t, "pr-123-original-uid", got["uid"])
	require.Equal(t, "PR-123 Original Title", got["title"])
	require.Nil(t, got["id"], "id must be null so Grafana keys the dashboard off its uid")
	require.Equal(t, float64(0), got["version"])
}

func TestPrepareDashboardForUploadKeepsOwnValuesWhenOverridesAreEmpty(t *testing.T) {
	raw := []byte(`{"uid":"original-uid","title":"Original Title"}`)

	prepared, err := PrepareDashboardForUpload(raw, "", "")
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(prepared, &got))
	require.Equal(t, "original-uid", got["uid"])
	require.Equal(t, "Original Title", got["title"])
}

func TestPrepareDashboardForUploadUnwrapsEnvelope(t *testing.T) {
	raw := []byte(`{"dashboard":{"uid":"original-uid","title":"Original Title","panels":[]},"folderUid":"ignored","overwrite":true}`)

	prepared, err := PrepareDashboardForUpload(raw, "pr-9-original-uid", "")
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(prepared, &got))
	require.Equal(t, "pr-9-original-uid", got["uid"])
	require.Equal(t, "Original Title", got["title"])
	require.NotContains(t, got, "dashboard", "the envelope must be unwrapped")
	require.NotContains(t, got, "folderUid", "envelope siblings must not reach the dashboard body")
	require.NotContains(t, got, "overwrite")
}

func TestPrepareDashboardForUploadPreservesUnknownFields(t *testing.T) {
	// A field this tool models nothing about, a plugin-specific block, and an
	// integer past 2^53 that a float64 round-trip would corrupt.
	raw := []byte(`{
		"uid": "original-uid",
		"title": "Original Title",
		"panels": [{"type": "custom-plugin-panel", "options": {"nested": {"deep": true}}}],
		"__inputs": [{"name": "DS_PROM", "pluginId": "prometheus"}],
		"someUnknownField": {"a": [1, 2, 3]},
		"bigNumber": 9007199254740993
	}`)

	prepared, err := PrepareDashboardForUpload(raw, "new-uid", "New Title")
	require.NoError(t, err)

	require.Contains(t, string(prepared), "9007199254740993",
		"large integers must survive without a float64 round-trip")

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(prepared, &got))
	require.JSONEq(t, `[{"type":"custom-plugin-panel","options":{"nested":{"deep":true}}}]`, string(got["panels"]))
	require.JSONEq(t, `[{"name":"DS_PROM","pluginId":"prometheus"}]`, string(got["__inputs"]))
	require.JSONEq(t, `{"a":[1,2,3]}`, string(got["someUnknownField"]))
}

func TestPrepareDashboardForUploadRejectsBadInput(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
	}{
		{name: "not JSON", raw: `not json`},
		{name: "not an object", raw: `[1, 2, 3]`},
		{name: "null", raw: `null`},
		{name: "multiple values", raw: `{"uid":"a"} {"uid":"b"}`},
		{name: "trailing data", raw: `{"uid":"a"} garbage`},
		{name: "envelope is not an object", raw: `{"dashboard": "a string"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PrepareDashboardForUpload([]byte(tt.raw), "uid", "title")
			require.Error(t, err)
		})
	}
}

func TestPrepareDashboardForUploadAllowsTrailingWhitespace(t *testing.T) {
	_, err := PrepareDashboardForUpload([]byte(`{"uid":"a"}`+"\n\n  "), "", "")
	require.NoError(t, err)
}

func TestGetDashboardStringField(t *testing.T) {
	raw := []byte(`{"uid":"the-uid","title":"The Title","version":3}`)

	uid, err := GetDashboardStringField(raw, "uid")
	require.NoError(t, err)
	require.Equal(t, "the-uid", uid)

	title, err := GetDashboardStringField(raw, "title")
	require.NoError(t, err)
	require.Equal(t, "The Title", title)

	missing, err := GetDashboardStringField(raw, "absent")
	require.NoError(t, err)
	require.Empty(t, missing, "an absent field reads as empty")

	notAString, err := GetDashboardStringField(raw, "version")
	require.NoError(t, err)
	require.Empty(t, notAString, "a non-string field reads as empty")
}

func TestGetDashboardStringFieldRejectsBadInput(t *testing.T) {
	_, err := GetDashboardStringField([]byte(`not json`), "uid")
	require.Error(t, err)
}

// The sync path must keep setting id to 0, not null, so that changing the
// upload path does not change what sync sends.
func TestNormalizeDashboardStillSetsIDToZero(t *testing.T) {
	normalized, err := normalizeDashboard([]byte(`{"uid":"a","id":41,"version":9}`))
	require.NoError(t, err)
	require.Contains(t, string(normalized), `"id":0`)
	require.False(t, strings.Contains(string(normalized), `"id":null`))
}
