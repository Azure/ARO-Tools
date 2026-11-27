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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestReconcileDatasourceEndpoints(t *testing.T) {
	for _, test := range []struct {
		name       string
		sourceName string
		currentURL string
		dryRun     bool
		status     int
		wantWrites int
		wantError  string
	}{
		{name: "stale endpoint", sourceName: "Managed_Prometheus_workspace", currentURL: "https://old.example.com", status: http.StatusOK, wantWrites: 1},
		{name: "unchanged endpoint", sourceName: "Managed_Prometheus_workspace", currentURL: "https://current.example.com", status: http.StatusOK},
		{name: "trailing slash", sourceName: "Managed_Prometheus_workspace", currentURL: "https://current.example.com/", status: http.StatusOK},
		{name: "dry run", sourceName: "Managed_Prometheus_workspace", currentURL: "https://old.example.com", dryRun: true, status: http.StatusOK},
		{name: "unmanaged source", sourceName: "workspace", currentURL: "https://old.example.com", status: http.StatusOK},
		{name: "undiscovered source", sourceName: "Managed_Prometheus_other", currentURL: "https://old.example.com", status: http.StatusOK},
		{name: "update rejected", sourceName: "Managed_Prometheus_workspace", currentURL: "https://old.example.com", status: http.StatusForbidden, wantWrites: 1, wantError: "HTTP 403"},
		{name: "concurrent update", sourceName: "Managed_Prometheus_workspace", currentURL: "https://old.example.com", status: http.StatusConflict, wantWrites: 1, wantError: "HTTP 409"},
	} {
		t.Run(test.name, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch {
				case request.URL.Path == "/api/datasources":
					require.NoError(t, json.NewEncoder(writer).Encode([]map[string]any{{"uid": "stable", "name": test.sourceName, "type": "prometheus", "url": test.currentURL}}))
				case request.URL.Path == "/api/datasources/uid/stable" && request.Method == http.MethodGet:
					_, err := io.WriteString(writer, `{"uid":"stable","name":"Managed_Prometheus_workspace","type":"prometheus","url":"https://old.example.com","access":"proxy","jsonData":{"azureAuth":true,"azureCredentials":{"authType":"msi"}},"secureJsonFields":{"password":true},"version":7,"futureOption":{"enabled":true}}`)
					require.NoError(t, err)
				case request.URL.Path == "/api/datasources/uid/stable" && request.Method == http.MethodPut:
					writes++
					var got map[string]any
					require.NoError(t, json.NewDecoder(request.Body).Decode(&got))
					var want map[string]any
					require.NoError(t, json.Unmarshal([]byte(`{"uid":"stable","name":"Managed_Prometheus_workspace","type":"prometheus","url":"https://current.example.com","access":"proxy","jsonData":{"azureAuth":true,"azureCredentials":{"authType":"msi"}},"version":7,"futureOption":{"enabled":true}}`), &want))
					if diff := cmp.Diff(want, got); diff != "" {
						t.Errorf("datasource update (-want +got):\n%s", diff)
					}
					writer.WriteHeader(test.status)
					_, err := io.WriteString(writer, `{}`)
					require.NoError(t, err)
				default:
					t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			err := newTestClient(t, server.URL).ReconcileDatasourceEndpoints(t.Context(), logr.Discard(), map[string]string{"workspace": "https://current.example.com"}, test.dryRun)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
			} else {
				require.NoError(t, err)
			}
			if diff := cmp.Diff(test.wantWrites, writes); diff != "" {
				t.Errorf("writes (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUpdateDatasourceEndpointRejectsChangedState(t *testing.T) {
	for _, test := range []struct {
		name      string
		body      string
		wantError string
	}{
		{
			name:      "renamed datasource",
			body:      `{"uid":"stable","name":"renamed","type":"prometheus","version":2}`,
			wantError: "identity changed",
		},
		{
			name:      "missing version",
			body:      `{"uid":"stable","name":"Managed_Prometheus_workspace","type":"prometheus"}`,
			wantError: "no valid version",
		},
		{
			name:      "malformed response",
			body:      `{`,
			wantError: "failed to decode datasource",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				require.Equal(t, http.MethodGet, request.Method)
				require.Equal(t, "Bearer test-token", request.Header.Get("Authorization"))
				writer.Header().Set("Content-Type", "application/json")
				_, err := io.WriteString(writer, test.body)
				require.NoError(t, err)
			}))
			defer server.Close()
			err := newTestClientWithToken(t, server.URL, "test-token").updateDatasourceEndpoint(t.Context(), Datasource{
				UID:  "stable",
				Name: "Managed_Prometheus_workspace",
				Type: "prometheus",
			}, "https://current.example.com")
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

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

func TestGetBaseURL(t *testing.T) {
	for _, tt := range []struct {
		name     string
		endpoint string
		want     string
	}{
		{name: "bare host", endpoint: "https://g.example.com", want: "https://g.example.com"},
		{name: "trailing slash", endpoint: "https://g.example.com/", want: "https://g.example.com"},
		{name: "reverse-proxy prefix", endpoint: "https://g.example.com/grafana", want: "https://g.example.com/grafana"},
		{name: "endpoint already ends in /api", endpoint: "https://g.example.com/api", want: "https://g.example.com"},
		{name: "schemeless host", endpoint: "g.example.com", want: "https://g.example.com"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, err := newClient(tt.endpoint, "")
			require.NoError(t, err)
			require.Equal(t, tt.want, client.GetBaseURL())
		})
	}
}

// Grafana's folder list endpoint defaults to far fewer folders than the
// instances grafanactl manages hold, so the limit must be sent explicitly.
func TestListFoldersRequestsAnExplicitLimit(t *testing.T) {
	var gotQueries []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/folders", request.URL.Path)
		gotQueries = append(gotQueries, request.URL.RawQuery)
		writeFolderSearchHits(t, writer, buildFolderUIDs(0, 42))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	folders, err := client.ListFolders(context.Background())
	require.NoError(t, err)

	require.Equal(t, []string{"limit=" + strconv.FormatInt(folderListLimit, 10)}, gotQueries,
		"the folder list must be one call carrying an explicit limit")
	require.Len(t, folders, 42)
	require.Equal(t, "folder-0", folders[0].UID)
	require.Equal(t, "folder-41", folders[len(folders)-1].UID)
	require.True(t, folders[0].Created.IsZero(),
		"the folder list endpoint reports no creation time")
}

func TestListFoldersHandlesEmptyInstance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeFolderSearchHits(t, writer, nil)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	folders, err := client.ListFolders(context.Background())
	require.NoError(t, err)
	require.Empty(t, folders)
}

// A full response means Grafana may have truncated the list. The caller gets
// what there is, plus a log line, rather than silently seeing a partial instance.
func TestListFoldersWarnsWhenTheLimitIsReached(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeFolderSearchHits(t, writer, buildFolderUIDs(0, int(folderListLimit)))
	}))
	defer server.Close()

	var logged []string
	logger := funcr.New(func(prefix, args string) {
		logged = append(logged, args)
	}, funcr.Options{})

	client := newTestClient(t, server.URL)
	folders, err := client.ListFolders(logr.NewContext(context.Background(), logger))
	require.NoError(t, err)

	require.Len(t, folders, int(folderListLimit))
	require.Contains(t, strings.Join(logged, "\n"), "may be incomplete")
}

func TestGetFolderByUID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/folders/pr-123", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, err := writer.Write([]byte(`{"id":7,"uid":"pr-123","title":"PR-123","created":"2026-01-20T14:03:21Z"}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	folder, err := client.GetFolderByUID(context.Background(), "pr-123")
	require.NoError(t, err)

	require.Equal(t, "pr-123", folder.UID)
	require.Equal(t, "PR-123", folder.Title)
	require.Equal(t, time.Date(2026, time.January, 20, 14, 3, 21, 0, time.UTC), folder.Created.UTC())
}

func TestGetFolderByUIDNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusNotFound, `{"message":"folder not found"}`)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetFolderByUID(context.Background(), "missing")
	require.ErrorIs(t, err, ErrFolderNotFound)
	require.Contains(t, err.Error(), "missing", "the error must name the folder")
}

// A 403 stands in for "the call failed for a reason other than absence". It is
// used in place of a 500 because the transport retries 5xx with a backoff,
// which would make the test take tens of seconds.
func TestGetFolderByUIDOtherErrorIsNotNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusForbidden, `{"message":"access denied"}`)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetFolderByUID(context.Background(), "pr-123")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrFolderNotFound,
		"a failed call must not be mistaken for an absent folder")
}

func TestDeleteFolderByUIDNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusNotFound, `{"message":"folder not found"}`)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteFolderByUID(context.Background(), "missing")
	require.ErrorIs(t, err, ErrFolderNotFound)
}

func TestDeleteFolderByUIDOtherErrorIsNotNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusForbidden, `{"message":"access denied"}`)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteFolderByUID(context.Background(), "folder-uid")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrFolderNotFound)
}

func TestCreateFolderSendsUID(t *testing.T) {
	for _, tt := range []struct {
		name        string
		uid         string
		wantRequest string
	}{
		{
			name:        "explicit uid",
			uid:         "pr-123",
			wantRequest: `{"title":"PR-123","uid":"pr-123"}`,
		},
		{
			name: "empty uid lets Grafana generate one",
			uid:  "",
			// An omitempty uid keeps the request identical to the one the
			// sync path sent before the uid parameter existed.
			wantRequest: `{"title":"PR-123"}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				require.Equal(t, "/api/folders", request.URL.Path)
				body, err := io.ReadAll(request.Body)
				require.NoError(t, err)
				gotBody = body

				writer.Header().Set("Content-Type", "application/json")
				_, err = writer.Write([]byte(`{"id":7,"uid":"pr-123","title":"PR-123","created":"2026-01-20T14:03:21Z"}`))
				require.NoError(t, err)
			}))
			defer server.Close()

			client := newTestClient(t, server.URL)
			folder, err := client.CreateFolder(context.Background(), tt.uid, "PR-123")
			require.NoError(t, err)

			require.JSONEq(t, tt.wantRequest, string(gotBody))
			require.Equal(t, "pr-123", folder.UID)
			require.Equal(t, time.Date(2026, time.January, 20, 14, 3, 21, 0, time.UTC), folder.Created.UTC())
		})
	}
}

func TestCreateFolderConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusConflict, `{"message":"a folder with the same uid already exists"}`)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreateFolder(context.Background(), "pr-123", "PR-123")
	require.ErrorIs(t, err, ErrFolderAlreadyExists)
}

func TestCreateFolderOtherErrorIsNotConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusForbidden, `{"message":"access denied"}`)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreateFolder(context.Background(), "pr-123", "PR-123")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrFolderAlreadyExists)
}

func buildFolderUIDs(offset, count int) []string {
	uids := make([]string, 0, count)
	for i := range count {
		uids = append(uids, fmt.Sprintf("folder-%d", offset+i))
	}
	return uids
}

func writeFolderSearchHits(t *testing.T, writer http.ResponseWriter, uids []string) {
	t.Helper()

	hits := make([]map[string]any, 0, len(uids))
	for _, uid := range uids {
		hits = append(hits, map[string]any{"uid": uid, "title": uid})
	}

	body, err := json.Marshal(hits)
	require.NoError(t, err)

	writer.Header().Set("Content-Type", "application/json")
	_, err = writer.Write(body)
	require.NoError(t, err)
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

func TestValidateUID(t *testing.T) {
	for _, tc := range []struct {
		name       string
		uid        string
		wantErrSub string
	}{
		{name: "empty is fine", uid: ""},
		{name: "well within the limit", uid: "pr-123"},
		{name: "underscores and mixed case", uid: "PR_123-abc"},
		{name: "exactly at the limit", uid: strings.Repeat("a", MaxUIDLength)},
		{name: "one over the limit", uid: strings.Repeat("a", MaxUIDLength+1), wantErrSub: "too long"},
		{name: "slash", uid: "feature/foo", wantErrSub: "invalid uid"},
		{name: "path traversal", uid: "../datasources", wantErrSub: "invalid uid"},
		{name: "space", uid: "pr 123", wantErrSub: "invalid uid"},
		{name: "percent", uid: "pr%2F123", wantErrSub: "invalid uid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateUID(tc.uid)
			if tc.wantErrSub != "" {
				require.ErrorContains(t, err, tc.wantErrSub)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
