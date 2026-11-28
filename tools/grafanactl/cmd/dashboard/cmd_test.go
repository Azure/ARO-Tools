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

package dashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/tools/grafanactl/cmd/base"
	"github.com/Azure/ARO-Tools/tools/grafanactl/internal/grafana"
)

// savedDashboard is the body Grafana receives on /api/dashboards/db.
type savedDashboard struct {
	Dashboard map[string]any `json:"dashboard"`
	FolderUID string         `json:"folderUid"`
	Overwrite bool           `json:"overwrite"`
}

func TestUpload(t *testing.T) {
	for _, test := range []struct {
		name      string
		file      string
		uid       string
		title     string
		folderUID string
		dryRun    bool
		wantPosts int
		wantUID   string
		wantTitle string
		wantError string
	}{
		{
			name:      "uploads the uid and title of the file",
			file:      `{"uid":"file-uid","title":"File Title","panels":[]}`,
			folderUID: "target-folder",
			wantPosts: 1,
			wantUID:   "file-uid",
			wantTitle: "File Title",
		},
		{
			name:      "uploads the overridden uid and title",
			file:      `{"uid":"file-uid","title":"File Title"}`,
			uid:       "override-uid",
			title:     "Override Title",
			folderUID: "target-folder",
			wantPosts: 1,
			wantUID:   "override-uid",
			wantTitle: "Override Title",
		},
		{
			name:      "uploads into the general folder",
			file:      `{"uid":"file-uid","title":"File Title"}`,
			wantPosts: 1,
			wantUID:   "file-uid",
			wantTitle: "File Title",
		},
		{
			name:      "missing uid",
			file:      `{"title":"File Title"}`,
			folderUID: "target-folder",
			wantError: "has no uid",
		},
		{
			name:      "missing title",
			file:      `{"uid":"file-uid"}`,
			folderUID: "target-folder",
			wantError: "has no title",
		},
		{
			name:      "invalid uid",
			file:      `{"uid":"not a valid uid","title":"File Title"}`,
			folderUID: "target-folder",
			wantError: "has an invalid uid",
		},
		{
			name:      "dry run",
			file:      `{"uid":"file-uid","title":"File Title"}`,
			uid:       "override-uid",
			folderUID: "target-folder",
			dryRun:    true,
			wantUID:   "override-uid",
			wantTitle: "File Title",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			posts := 0
			var got savedDashboard
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.URL.Path != "/api/dashboards/db" {
					t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
					writer.WriteHeader(http.StatusNotFound)
					return
				}
				posts++
				require.NoError(t, json.NewDecoder(request.Body).Decode(&got))
				writer.Header().Set("Content-Type", "application/json")
				_, err := io.WriteString(writer, `{"status":"success","uid":"file-uid","version":1}`)
				require.NoError(t, err)
			}))
			defer server.Close()

			file := filepath.Join(t.TempDir(), "dashboard.json")
			require.NoError(t, os.WriteFile(file, []byte(test.file), 0o600))

			opts := buildTestUploadOptions(t, server.URL, &RawUploadOptions{
				BaseOptions: &base.BaseOptions{DryRun: test.dryRun},
				File:        file,
				FolderUID:   test.folderUID,
				UID:         test.uid,
				Title:       test.title,
			})

			info, err := opts.Upload(t.Context())
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Equal(t, 0, posts)
				return
			}

			require.NoError(t, err)
			require.Equal(t, test.wantPosts, posts)
			require.Equal(t, DashboardInfo{
				UID:   test.wantUID,
				Title: test.wantTitle,
				URL:   server.URL + "/d/" + test.wantUID,
			}, info)

			if test.wantPosts == 0 {
				return
			}
			require.Equal(t, test.folderUID, got.FolderUID)
			require.True(t, got.Overwrite, "uploads replace the dashboard held under the same uid")
			require.Equal(t, test.wantUID, got.Dashboard["uid"])
			require.Equal(t, test.wantTitle, got.Dashboard["title"])
		})
	}
}

func TestUploadReportsAMissingFile(t *testing.T) {
	opts := buildTestUploadOptions(t, "https://grafana.example.com", &RawUploadOptions{
		BaseOptions: &base.BaseOptions{},
		File:        filepath.Join(t.TempDir(), "absent.json"),
	})

	_, err := opts.Upload(t.Context())
	require.ErrorContains(t, err, "failed to read dashboard file")
}

// buildTestUploadOptions builds upload options whose Grafana client targets the
// given test server. An empty token is passed because these tests exercise the
// request bodies, not auth; the test servers do not validate the Authorization
// header.
func buildTestUploadOptions(t *testing.T, serverURL string, raw *RawUploadOptions) *CompletedUploadOptions {
	t.Helper()

	client, err := grafana.NewClientForEndpoint(serverURL, "")
	require.NoError(t, err)

	return &CompletedUploadOptions{
		validatedUploadOptions: &validatedUploadOptions{
			RawUploadOptions:     raw,
			CompletedBaseOptions: &base.CompletedBaseOptions{},
		},
		GrafanaClient: client,
	}
}
