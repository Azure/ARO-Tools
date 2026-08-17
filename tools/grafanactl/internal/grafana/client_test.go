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
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newTestClient builds a Client whose backend targets the given test server.
// An empty token is passed because these tests exercise request/response bodies,
// not auth; the test servers do not validate the Authorization header.
func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	return newTestClientWithToken(t, serverURL, "")
}

func newTestClientWithToken(t *testing.T, serverURL, token string) *Client {
	t.Helper()
	client, err := newClient(serverURL, token)
	require.NoError(t, err)
	return client
}

func TestParseGrafanaAPILocation(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		scheme   string
		host     string
		basePath string
	}{
		{
			name:     "bare https host",
			endpoint: "https://g.example.com",
			scheme:   "https",
			host:     "g.example.com",
			basePath: "/api",
		},
		{
			name:     "trailing slash",
			endpoint: "https://g.example.com/",
			scheme:   "https",
			host:     "g.example.com",
			basePath: "/api",
		},
		{
			name:     "reverse-proxy prefix",
			endpoint: "https://g.example.com/grafana",
			scheme:   "https",
			host:     "g.example.com",
			basePath: "/grafana/api",
		},
		{
			name:     "reverse-proxy prefix with trailing slash",
			endpoint: "https://g.example.com/grafana/",
			scheme:   "https",
			host:     "g.example.com",
			basePath: "/grafana/api",
		},
		{
			name:     "endpoint already ends in /api",
			endpoint: "https://g.example.com/api",
			scheme:   "https",
			host:     "g.example.com",
			basePath: "/api",
		},
		{
			name:     "endpoint already ends in /api/",
			endpoint: "https://g.example.com/api/",
			scheme:   "https",
			host:     "g.example.com",
			basePath: "/api",
		},
		{
			name:     "schemeless host defaults to https",
			endpoint: "g.example.com",
			scheme:   "https",
			host:     "g.example.com",
			basePath: "/api",
		},
		{
			name:     "http test server",
			endpoint: "http://127.0.0.1:3000",
			scheme:   "http",
			host:     "127.0.0.1:3000",
			basePath: "/api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := parseGrafanaAPILocation(tt.endpoint)
			require.NoError(t, err)
			require.Equal(t, tt.scheme, loc.Scheme)
			require.Equal(t, tt.host, loc.Host)
			require.Equal(t, tt.basePath, loc.BasePath)
		})
	}
}

func TestParseGrafanaAPILocationRejectsEmpty(t *testing.T) {
	_, err := parseGrafanaAPILocation("")
	require.Error(t, err)
}

func TestClientSendsBearerToken(t *testing.T) {
	t.Run("openapi", func(t *testing.T) {
		var gotAuth string
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			gotAuth = request.Header.Get("Authorization")
			writer.Header().Set("Content-Type", "application/json")
			_, err := writer.Write([]byte(`[]`))
			require.NoError(t, err)
		}))
		defer server.Close()

		client := newTestClientWithToken(t, server.URL, "test-token")
		_, err := client.ListFolders(context.Background())
		require.NoError(t, err)
		require.Equal(t, "Bearer test-token", gotAuth)
	})

	t.Run("raw dashboard get", func(t *testing.T) {
		var gotAuth string
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			gotAuth = request.Header.Get("Authorization")
			writer.Header().Set("Content-Type", "application/json")
			_, err := writer.Write([]byte(`{"dashboard":` + dashboardJSON + `,"meta":{"folderUid":"folder-uid"}}`))
			require.NoError(t, err)
		}))
		defer server.Close()

		client := newTestClientWithToken(t, server.URL, "test-token")
		_, _, err := client.GetRawDashboardByUID(context.Background(), "test-dashboard")
		require.NoError(t, err)
		require.Equal(t, "Bearer test-token", gotAuth)
	})
}

func TestSetRawDashboardPreservesUnknownFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/dashboards/db", request.URL.Path)

		var payload struct {
			Dashboard json.RawMessage `json:"dashboard"`
			FolderUID string          `json:"folderUid"`
			Overwrite bool            `json:"overwrite"`
		}
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &payload))
		require.Equal(t, "folder-uid", payload.FolderUID)
		require.True(t, payload.Overwrite)

		var keys map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &keys))
		gotKeys := make([]string, 0, len(keys))
		for k := range keys {
			gotKeys = append(gotKeys, k)
		}
		require.ElementsMatch(t, []string{"dashboard", "folderUid", "overwrite"}, gotKeys,
			"POST body must not include generated OpenAPI fields such as UpdatedAt")

		// The dashboard body must be uploaded byte-for-byte (lossless), so it
		// JSON-equals the input including fields the client does not model.
		require.JSONEq(t, dashboardJSON, string(payload.Dashboard))

		writer.Header().Set("Content-Type", "application/json")
		_, err = writer.Write([]byte(`{"status":"success","uid":"test-dashboard"}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	require.NoError(t, client.SetRawDashboard(context.Background(), []byte(dashboardJSON), "folder-uid", true))
}

func TestGetRawDashboardPreservesUnknownFields(t *testing.T) {
	// Include an integer larger than 2^53 so a float64 decode+re-marshal would
	// change the value. The GET path must keep the original JSON digits.
	rawDashboard := `{"uid":"test-dashboard","title":"Test Dashboard","largeInteger":9007199254740993,"panels":[{"transformations":[{"id":"renameByRegex"}],"pluginSpecificField":{"preserve":true}}]}`

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/dashboards/uid/test-dashboard", request.URL.Path)

		writer.Header().Set("Content-Type", "application/json")
		_, err := writer.Write([]byte(`{"dashboard":` + rawDashboard + `,"meta":{"folderUid":"folder-uid"}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	raw, properties, err := client.GetRawDashboardByUID(context.Background(), "test-dashboard")
	require.NoError(t, err)
	require.Equal(t, "folder-uid", properties.FolderUID)
	require.JSONEq(t, rawDashboard, string(raw))

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var dashboard map[string]interface{}
	require.NoError(t, decoder.Decode(&dashboard))
	require.Equal(t, json.Number("9007199254740993"), dashboard["largeInteger"])
	panel := dashboard["panels"].([]interface{})[0].(map[string]interface{})
	require.Contains(t, panel, "transformations")
	require.Contains(t, panel, "pluginSpecificField")
}

func TestGetRawDashboardNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/dashboards/uid/missing", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNotFound)
		_, err := writer.Write([]byte(`{"message":"dashboard not found","dashboard":{"secret":"should-not-appear-in-error"}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, _, err := client.GetRawDashboardByUID(context.Background(), "missing")
	require.Error(t, err)
	require.ErrorContains(t, err, `failed to get dashboard "missing": status 404`)
	require.NotContains(t, err.Error(), "should-not-appear-in-error")
}

func TestDecodeDashboardEnvelopeRejectsOversizedBody(t *testing.T) {
	body := `{"dashboard":{"uid":"x","title":"too-big"},"meta":{"slug":"x"}}`
	_, _, err := decodeDashboardEnvelope(strings.NewReader(body), "x", 8)
	require.ErrorContains(t, err, `dashboard "x": response exceeds 8 bytes`)
}

func TestDecodeDashboardEnvelopeAcceptsBodyAtMaxBytes(t *testing.T) {
	body := `{"dashboard":{"uid":"x","title":"ok"},"meta":{"folderUid":"folder-x"}}`
	raw, _, err := decodeDashboardEnvelope(strings.NewReader(body), "x", int64(len(body)))
	require.NoError(t, err)
	require.JSONEq(t, `{"uid":"x","title":"ok"}`, string(raw))
}

func TestDecodeDashboardEnvelopeDecodesWithoutBufferingTwice(t *testing.T) {
	body := `{"dashboard":{"uid":"x","title":"ok","keepMe":true},"meta":{"folderUid":"folder-x"}}`
	raw, props, err := decodeDashboardEnvelope(strings.NewReader(body), "x", 1024)
	require.NoError(t, err)
	require.JSONEq(t, `{"uid":"x","title":"ok","keepMe":true}`, string(raw))
	require.Equal(t, "folder-x", props.FolderUID)
}

func TestDecodeDashboardEnvelopeAllowsTrailingWhitespace(t *testing.T) {
	body := `{"dashboard":{"uid":"x","title":"ok"},"meta":{"folderUid":"folder-x"}}` + "  \n"
	raw, _, err := decodeDashboardEnvelope(strings.NewReader(body), "x", 1024)
	require.NoError(t, err)
	require.JSONEq(t, `{"uid":"x","title":"ok"}`, string(raw))
}

func TestDecodeDashboardEnvelopeRejectsTrailingJSON(t *testing.T) {
	body := `{"dashboard":{"uid":"x","title":"ok"},"meta":{}}{"extra":true}`
	_, _, err := decodeDashboardEnvelope(strings.NewReader(body), "x", 1024)
	require.ErrorContains(t, err, `dashboard "x": response contains multiple JSON values`)
}
