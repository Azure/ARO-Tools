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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/tools/grafanactl/internal/grafana"
)

func TestNewDashboardCommandSubcommands(t *testing.T) {
	cmd, err := NewDashboardCommand("main")
	require.NoError(t, err)

	got := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		got = append(got, sub.Name())
	}
	require.ElementsMatch(t, []string{"upload"}, got)
}

func TestUploadFlags(t *testing.T) {
	cmd, err := NewDashboardCommand("main")
	require.NoError(t, err)

	var upload *cobra.Command
	for _, sub := range cmd.Commands() {
		if sub.Name() == "upload" {
			upload = sub
		}
	}
	require.NotNil(t, upload)

	for _, name := range []string{"file", "folder-uid", "uid", "title", "dry-run"} {
		require.NotNil(t, upload.Flags().Lookup(name), "flag --%s must exist", name)
	}

	file := upload.Flags().Lookup("file")
	require.Contains(t, file.Annotations[cobra.BashCompOneRequiredFlag], "true",
		"--file must be required")

	for _, name := range []string{"folder-uid", "uid", "title"} {
		require.NotContains(t, upload.Flags().Lookup(name).Annotations[cobra.BashCompOneRequiredFlag], "true",
			"--%s must be optional", name)
	}
}

func TestValidateResolvesFileToAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"uid":"a","title":"A"}`), 0o600))

	opts := buildCompleteOptions(t)
	opts.File = getRelativePath(t, path)

	validated, err := opts.Validate(t.Context())
	require.NoError(t, err)
	require.Equal(t, path, validated.File, "the file path must be made absolute")
}

func TestValidateRejectsMissingFile(t *testing.T) {
	opts := buildCompleteOptions(t)
	opts.File = filepath.Join(t.TempDir(), "absent.json")

	_, err := opts.Validate(t.Context())
	require.ErrorContains(t, err, "dashboard file not found")
}

func TestValidateRejectsEmptyFile(t *testing.T) {
	opts := buildCompleteOptions(t)
	opts.File = ""

	_, err := opts.Validate(t.Context())
	require.ErrorContains(t, err, "dashboard file is required")
}

func TestValidateAcceptsEmptyUIDAndFolderUID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"uid":"a","title":"A"}`), 0o600))

	opts := buildCompleteOptions(t)
	opts.File = path
	opts.UID = ""
	opts.FolderUID = ""

	_, err := opts.Validate(t.Context())
	require.NoError(t, err, "an empty uid keeps the file's own, and an empty folder-uid uploads to General")
}

func TestValidateRejectsUIDTooLong(t *testing.T) {
	for _, tc := range []struct {
		name string
	}{
		{name: "uid"},
		{name: "folder-uid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "dashboard.json")
			require.NoError(t, os.WriteFile(path, []byte(`{"uid":"a","title":"A"}`), 0o600))

			opts := buildCompleteOptions(t)
			opts.File = path
			tooLong := strings.Repeat("a", grafana.MaxUIDLength+1)
			if tc.name == "uid" {
				opts.UID = tooLong
			} else {
				opts.FolderUID = tooLong
			}

			_, err := opts.Validate(t.Context())
			require.ErrorContains(t, err, "too long")
		})
	}
}

func TestValidateRejectsBadBaseOptions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"uid":"a"}`), 0o600))

	opts := BuildDefaultUploadOptions()
	opts.File = path

	_, err := opts.Validate(t.Context())
	require.ErrorContains(t, err, "subscription ID, resource group, and grafana name are required")
}

func TestMaxUIDLengthMatchesGrafana(t *testing.T) {
	require.Equal(t, grafana.MaxUIDLength, MaxUIDLength,
		"the re-exported limit must not drift from the Grafana client's")
}

func TestValidateUID(t *testing.T) {
	require.NoError(t, ValidateUID(""), "an empty uid keeps the document's own")
	require.NoError(t, ValidateUID(strings.Repeat("a", MaxUIDLength)))
	require.ErrorContains(t, ValidateUID(strings.Repeat("a", MaxUIDLength+1)), "too long")
	require.ErrorContains(t, ValidateUID("has spaces"), "invalid uid")
}

func TestReadDashboardIdentity(t *testing.T) {
	uid, title, err := GetDashboardIdentity([]byte(`{"uid":"the-uid","title":"The Title"}`))
	require.NoError(t, err)
	require.Equal(t, "the-uid", uid)
	require.Equal(t, "The Title", title)

	uid, title, err = GetDashboardIdentity([]byte(`{"dashboard":{"uid":"the-uid","title":"The Title"}}`))
	require.NoError(t, err)
	require.Equal(t, "the-uid", uid, "an exported document must be unwrapped")
	require.Equal(t, "The Title", title)

	_, _, err = GetDashboardIdentity([]byte(`not json`))
	require.Error(t, err)
}

func TestBuildDashboardURL(t *testing.T) {
	require.Equal(t,
		"https://g.example.com/d/pr-123-overview",
		BuildDashboardURL("https://g.example.com", "pr-123-overview"))
}

func buildCompleteOptions(t *testing.T) *RawUploadOptions {
	t.Helper()

	opts := BuildDefaultUploadOptions()
	opts.SubscriptionID = "00000000-0000-0000-0000-000000000000"
	opts.ResourceGroup = "global"
	opts.GrafanaName = "arohcp-dev"
	return opts
}

// getRelativePath expresses path relative to the working directory, so that
// Validate has something to make absolute.
func getRelativePath(t *testing.T, path string) string {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err)

	rel, err := filepath.Rel(wd, path)
	require.NoError(t, err)
	return rel
}
