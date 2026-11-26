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

package folder

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/tools/grafanactl/cmd/base"
	"github.com/Azure/ARO-Tools/tools/grafanactl/internal/grafana"
)

func TestCreate(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	for _, test := range []struct {
		name   string
		uid    string
		title  string
		dryRun bool
		// getStatus is the status of the folder lookup that precedes the
		// create. postStatus and secondGetStatus, when set, are the status of
		// the create and of the lookup that follows a create conflict.
		getStatus       int
		postStatus      int
		secondGetStatus int
		wantPosts       int
		wantFolder      FolderInfo
		wantCreated     time.Time
		wantError       string
	}{
		{
			name:      "existing folder is left alone",
			uid:       "folder-uid",
			title:     "New Title",
			getStatus: http.StatusOK,
			wantFolder: FolderInfo{
				UID:     "folder-uid",
				Title:   "Existing Title",
				Created: created,
			},
		},
		{
			name:      "folder is created when absent",
			uid:       "folder-uid",
			title:     "New Title",
			getStatus: http.StatusNotFound,
			wantPosts: 1,
			wantFolder: FolderInfo{
				UID:   "folder-uid",
				Title: "New Title",
			},
		},
		{
			name:            "folder created by another writer is read back",
			uid:             "folder-uid",
			title:           "New Title",
			getStatus:       http.StatusNotFound,
			postStatus:      http.StatusConflict,
			secondGetStatus: http.StatusOK,
			wantPosts:       1,
			wantFolder: FolderInfo{
				UID:     "folder-uid",
				Title:   "Existing Title",
				Created: created,
			},
		},
		{
			name:            "create conflict with a failing read back is propagated",
			uid:             "folder-uid",
			title:           "New Title",
			getStatus:       http.StatusNotFound,
			postStatus:      http.StatusConflict,
			secondGetStatus: http.StatusForbidden,
			wantPosts:       1,
			wantError:       "failed to create folder",
		},
		{
			name:      "lookup failure is propagated",
			uid:       "folder-uid",
			title:     "New Title",
			getStatus: http.StatusForbidden,
			wantError: "failed to get folder",
		},
		{
			name:      "dry run creates nothing",
			uid:       "folder-uid",
			title:     "New Title",
			dryRun:    true,
			getStatus: http.StatusNotFound,
			wantFolder: FolderInfo{
				UID:   "folder-uid",
				Title: "New Title",
			},
		},
		{
			name:      "missing uid",
			title:     "New Title",
			wantError: "folder UID is required",
		},
		{
			name:      "missing title",
			uid:       "folder-uid",
			wantError: "folder title is required",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gets, posts := 0, 0
			var posted map[string]string
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch {
				case request.Method == http.MethodGet && request.URL.Path == "/api/folders/folder-uid":
					gets++
					status := test.getStatus
					if gets > 1 && test.secondGetStatus != 0 {
						status = test.secondGetStatus
					}
					writer.WriteHeader(status)
					if status != http.StatusOK {
						_, err := io.WriteString(writer, `{"message":"folder lookup failed"}`)
						require.NoError(t, err)
						return
					}
					_, err := io.WriteString(writer, `{"uid":"folder-uid","title":"Existing Title","created":"2026-01-02T03:04:05Z"}`)
					require.NoError(t, err)
				case request.Method == http.MethodPost && request.URL.Path == "/api/folders":
					posts++
					require.NoError(t, json.NewDecoder(request.Body).Decode(&posted))
					if test.postStatus != 0 && test.postStatus != http.StatusOK {
						writer.WriteHeader(test.postStatus)
						_, err := io.WriteString(writer, `{"message":"a folder with the same uid already exists"}`)
						require.NoError(t, err)
						return
					}
					_, err := io.WriteString(writer, `{"uid":"folder-uid","title":"New Title"}`)
					require.NoError(t, err)
				default:
					t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			opts := buildTestFolderOptions(t, server.URL, &RawFolderOptions{
				BaseOptions: &base.BaseOptions{DryRun: test.dryRun},
				UID:         test.uid,
				Title:       test.title,
			})

			folder, err := opts.Create(t.Context())
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Equal(t, test.wantPosts, posts)
				return
			}

			require.NoError(t, err)
			require.Equal(t, test.wantPosts, posts)

			want := test.wantFolder
			want.URL = BuildFolderURL(server.URL, want.UID)
			require.Equal(t, want, folder)

			if test.wantPosts > 0 {
				require.Equal(t, map[string]string{"uid": test.uid, "title": test.title}, posted)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	for _, test := range []struct {
		name         string
		uid          string
		dryRun       bool
		deleteStatus int
		wantDeletes  int
		wantError    string
	}{
		{
			name:         "folder is deleted",
			uid:          "folder-uid",
			deleteStatus: http.StatusOK,
			wantDeletes:  1,
		},
		{
			name:         "absent folder is a success",
			uid:          "folder-uid",
			deleteStatus: http.StatusNotFound,
			wantDeletes:  1,
		},
		{
			name:         "delete failure is propagated",
			uid:          "folder-uid",
			deleteStatus: http.StatusForbidden,
			wantDeletes:  1,
			wantError:    "failed to delete folder",
		},
		{
			name:         "dry run deletes nothing",
			uid:          "folder-uid",
			dryRun:       true,
			deleteStatus: http.StatusOK,
		},
		{
			name:      "missing uid",
			wantError: "folder UID is required",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			deletes := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodDelete || request.URL.Path != "/api/folders/folder-uid" {
					t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
					writer.WriteHeader(http.StatusNotFound)
					return
				}
				deletes++
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(test.deleteStatus)
				_, err := io.WriteString(writer, `{"message":"Folder deleted"}`)
				require.NoError(t, err)
			}))
			defer server.Close()

			opts := buildTestFolderOptions(t, server.URL, &RawFolderOptions{
				BaseOptions: &base.BaseOptions{DryRun: test.dryRun},
				UID:         test.uid,
			})

			err := opts.Delete(t.Context())
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.wantDeletes, deletes)
		})
	}
}

// buildTestFolderOptions builds folder options whose Grafana client targets the
// given test server. An empty token is passed because these tests exercise the
// requests and responses, not auth; the test servers do not validate the
// Authorization header.
func buildTestFolderOptions(t *testing.T, serverURL string, raw *RawFolderOptions) *CompletedFolderOptions {
	t.Helper()

	client, err := grafana.NewClientForEndpoint(serverURL, "")
	require.NoError(t, err)

	return &CompletedFolderOptions{
		validatedFolderOptions: &validatedFolderOptions{
			RawFolderOptions:     raw,
			CompletedBaseOptions: &base.CompletedBaseOptions{},
		},
		GrafanaClient: client,
	}
}
