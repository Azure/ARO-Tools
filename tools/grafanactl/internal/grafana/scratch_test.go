// Copyright 2025 Microsoft Corporation
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
	"fmt"
	"testing"
	"time"

	"github.com/grafana-tools/sdk"

	"github.com/Azure/ARO-Tools/tools/grafanactl/config"
)

type mockScratchClient struct {
	folders              []sdk.Folder
	dashboards           []sdk.FoundBoard
	searchFolders        []sdk.FoundBoard
	dashboardsByUID      map[string]sdk.BoardProperties
	createdFolders       []string
	updatedPermissions   map[string][]sdk.FolderPermission
	deletedDashboardUIDs []string
	deletedFolderUIDs    []string

	listFoldersErr     error
	createFolderErr    error
	updatePermsErr     error
	listDashboardsErr  error
	searchFoldersErr   error
	getDashboardErr    error
	deleteDashboardErr error
	deleteFolderErr    error
}

func newMockScratchClient() *mockScratchClient {
	return &mockScratchClient{
		dashboardsByUID:    make(map[string]sdk.BoardProperties),
		updatedPermissions: make(map[string][]sdk.FolderPermission),
	}
}

func (m *mockScratchClient) ListFolders(_ context.Context) ([]sdk.Folder, error) {
	if m.listFoldersErr != nil {
		return nil, m.listFoldersErr
	}
	return m.folders, nil
}

func (m *mockScratchClient) CreateFolder(_ context.Context, title string) (sdk.Folder, error) {
	if m.createFolderErr != nil {
		return sdk.Folder{}, m.createFolderErr
	}
	m.createdFolders = append(m.createdFolders, title)
	f := sdk.Folder{Title: title, UID: "uid-" + title, ID: len(m.folders) + 1}
	m.folders = append(m.folders, f)
	return f, nil
}

func (m *mockScratchClient) UpdateFolderPermissions(_ context.Context, folderUID string, permissions ...sdk.FolderPermission) error {
	if m.updatePermsErr != nil {
		return m.updatePermsErr
	}
	m.updatedPermissions[folderUID] = permissions
	return nil
}

func (m *mockScratchClient) ListDashboards(_ context.Context) ([]sdk.FoundBoard, error) {
	if m.listDashboardsErr != nil {
		return nil, m.listDashboardsErr
	}
	return m.dashboards, nil
}

func (m *mockScratchClient) SearchFolders(_ context.Context) ([]sdk.FoundBoard, error) {
	if m.searchFoldersErr != nil {
		return nil, m.searchFoldersErr
	}
	return m.searchFolders, nil
}

func (m *mockScratchClient) GetDashboardByUID(_ context.Context, uid string) (sdk.Board, sdk.BoardProperties, error) {
	if m.getDashboardErr != nil {
		return sdk.Board{}, sdk.BoardProperties{}, m.getDashboardErr
	}
	props, ok := m.dashboardsByUID[uid]
	if !ok {
		return sdk.Board{}, sdk.BoardProperties{}, fmt.Errorf("dashboard %q not found", uid)
	}
	return sdk.Board{UID: uid}, props, nil
}

func (m *mockScratchClient) DeleteDashboardByUID(_ context.Context, uid string) error {
	if m.deleteDashboardErr != nil {
		return m.deleteDashboardErr
	}
	m.deletedDashboardUIDs = append(m.deletedDashboardUIDs, uid)
	return nil
}

func (m *mockScratchClient) DeleteFolderByUID(_ context.Context, uid string) error {
	if m.deleteFolderErr != nil {
		return m.deleteFolderErr
	}
	m.deletedFolderUIDs = append(m.deletedFolderUIDs, uid)
	return nil
}

func TestSyncScratchFolders_CreatesFolder(t *testing.T) {
	client := newMockScratchClient()
	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.createdFolders) != 1 || client.createdFolders[0] != "Scratchpad" {
		t.Fatalf("expected folder 'Scratchpad' to be created, got %v", client.createdFolders)
	}
}

func TestSyncScratchFolders_ReuseExistingFolder(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "existing-uid", ID: 42}}
	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.createdFolders) != 0 {
		t.Fatalf("expected no folder creation, got %v", client.createdFolders)
	}

	perms, ok := client.updatedPermissions["existing-uid"]
	if !ok {
		t.Fatal("expected permissions to be set on existing-uid")
	}
	if len(perms) != 3 {
		t.Fatalf("expected 3 permission entries, got %d", len(perms))
	}
}

func TestSyncScratchFolders_SetsPermissions(t *testing.T) {
	client := newMockScratchClient()
	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	folderUID := "uid-Scratchpad"
	perms := client.updatedPermissions[folderUID]
	if len(perms) != 3 {
		t.Fatalf("expected 3 permission entries, got %d", len(perms))
	}

	expected := map[string]sdk.PermissionType{
		"Viewer": sdk.PermissionEdit,
		"Editor": sdk.PermissionEdit,
		"Admin":  sdk.PermissionAdmin,
	}
	for _, p := range perms {
		want, ok := expected[p.Role]
		if !ok {
			t.Errorf("unexpected role %q in permissions", p.Role)
			continue
		}
		if p.Permission != want {
			t.Errorf("role %q: got permission %d, want %d", p.Role, p.Permission, want)
		}
	}
}

func TestSyncScratchFolders_DeletesExpiredDashboards(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "scratch-uid"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	client.dashboards = []sdk.FoundBoard{
		{UID: "old-dash", Title: "Old Dashboard", FolderUID: "scratch-uid"},
		{UID: "new-dash", Title: "New Dashboard", FolderUID: "scratch-uid"},
		{UID: "other-dash", Title: "Other Dashboard", FolderUID: "other-folder"},
	}
	client.dashboardsByUID["old-dash"] = sdk.BoardProperties{
		Created: now.Add(-8 * 24 * time.Hour),
	}
	client.dashboardsByUID["new-dash"] = sdk.BoardProperties{
		Created: now.Add(-1 * 24 * time.Hour),
	}

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.deletedDashboardUIDs) != 1 {
		t.Fatalf("expected 1 deletion, got %d: %v", len(client.deletedDashboardUIDs), client.deletedDashboardUIDs)
	}
	if client.deletedDashboardUIDs[0] != "old-dash" {
		t.Fatalf("expected 'old-dash' to be deleted, got %q", client.deletedDashboardUIDs[0])
	}
}

func TestSyncScratchFolders_DeletesExpiredDashboardsInSubfolders(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "scratch-uid"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	client.searchFolders = []sdk.FoundBoard{
		{UID: "scratch-uid", Title: "Scratchpad", Type: "dash-folder"},
		{UID: "sub-1", Title: "subfolder1", Type: "dash-folder", FolderUID: "scratch-uid"},
	}
	client.dashboards = []sdk.FoundBoard{
		{UID: "root-dash", Title: "Root Dashboard", FolderUID: "scratch-uid"},
		{UID: "sub-dash", Title: "Sub Dashboard", FolderUID: "sub-1"},
	}
	client.dashboardsByUID["root-dash"] = sdk.BoardProperties{Created: now.Add(-8 * 24 * time.Hour)}
	client.dashboardsByUID["sub-dash"] = sdk.BoardProperties{Created: now.Add(-8 * 24 * time.Hour)}

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.deletedDashboardUIDs) != 2 {
		t.Fatalf("expected 2 deletions, got %d: %v", len(client.deletedDashboardUIDs), client.deletedDashboardUIDs)
	}
}

func TestSyncScratchFolders_DeletesEmptySubfolders(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "scratch-uid"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	client.searchFolders = []sdk.FoundBoard{
		{UID: "scratch-uid", Title: "Scratchpad", Type: "dash-folder"},
		{UID: "empty-sub", Title: "empty", Type: "dash-folder", FolderUID: "scratch-uid"},
	}
	// No dashboards anywhere
	client.dashboards = nil

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.deletedFolderUIDs) != 1 || client.deletedFolderUIDs[0] != "empty-sub" {
		t.Fatalf("expected empty-sub to be deleted, got %v", client.deletedFolderUIDs)
	}
}

func TestSyncScratchFolders_DeletesNestedEmptySubfoldersLeafFirst(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "scratch-uid"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	client.searchFolders = []sdk.FoundBoard{
		{UID: "scratch-uid", Title: "Scratchpad", Type: "dash-folder"},
		{UID: "parent-sub", Title: "parent", Type: "dash-folder", FolderUID: "scratch-uid"},
		{UID: "child-sub", Title: "child", Type: "dash-folder", FolderUID: "parent-sub"},
	}
	client.dashboards = nil

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.deletedFolderUIDs) != 2 {
		t.Fatalf("expected 2 folder deletions, got %d: %v", len(client.deletedFolderUIDs), client.deletedFolderUIDs)
	}
	// Leaf first
	if client.deletedFolderUIDs[0] != "child-sub" {
		t.Fatalf("expected child-sub deleted first, got %q", client.deletedFolderUIDs[0])
	}
	if client.deletedFolderUIDs[1] != "parent-sub" {
		t.Fatalf("expected parent-sub deleted second, got %q", client.deletedFolderUIDs[1])
	}
}

func TestSyncScratchFolders_KeepsNonEmptySubfolders(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "scratch-uid"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	client.searchFolders = []sdk.FoundBoard{
		{UID: "scratch-uid", Title: "Scratchpad", Type: "dash-folder"},
		{UID: "has-dash-sub", Title: "hasdash", Type: "dash-folder", FolderUID: "scratch-uid"},
	}
	client.dashboards = []sdk.FoundBoard{
		{UID: "fresh-dash", Title: "Fresh Dashboard", FolderUID: "has-dash-sub"},
	}
	client.dashboardsByUID["fresh-dash"] = sdk.BoardProperties{
		Created: now.Add(-1 * time.Hour), // not expired
	}

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.deletedFolderUIDs) != 0 {
		t.Fatalf("expected no folder deletions, got %v", client.deletedFolderUIDs)
	}
}

func TestSyncScratchFolders_DoesNotDeleteRootScratchFolder(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "scratch-uid"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	client.searchFolders = []sdk.FoundBoard{
		{UID: "scratch-uid", Title: "Scratchpad", Type: "dash-folder"},
	}
	client.dashboards = nil

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, uid := range client.deletedFolderUIDs {
		if uid == "scratch-uid" {
			t.Fatal("root scratch folder should not be deleted")
		}
	}
}

func TestSyncScratchFolders_ExpiryBoundary(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "scratch-uid"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)
	maxAge := 168 * time.Hour

	client.dashboards = []sdk.FoundBoard{
		{UID: "exactly-at-boundary", Title: "Boundary", FolderUID: "scratch-uid"},
		{UID: "one-second-before", Title: "Just Expired", FolderUID: "scratch-uid"},
		{UID: "one-second-after", Title: "Not Expired", FolderUID: "scratch-uid"},
	}
	client.dashboardsByUID["exactly-at-boundary"] = sdk.BoardProperties{
		Created: now.Add(-maxAge),
	}
	client.dashboardsByUID["one-second-before"] = sdk.BoardProperties{
		Created: now.Add(-maxAge - time.Second),
	}
	client.dashboardsByUID["one-second-after"] = sdk.BoardProperties{
		Created: now.Add(-maxAge + time.Second),
	}

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.deletedDashboardUIDs) != 1 {
		t.Fatalf("expected 1 deletion, got %d: %v", len(client.deletedDashboardUIDs), client.deletedDashboardUIDs)
	}
	if client.deletedDashboardUIDs[0] != "one-second-before" {
		t.Fatalf("expected 'one-second-before' to be deleted, got %q", client.deletedDashboardUIDs[0])
	}
}

func TestSyncScratchFolders_DryRun(t *testing.T) {
	client := newMockScratchClient()
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	client.searchFolders = []sdk.FoundBoard{
		{UID: "dry-run-Scratchpad", Title: "Scratchpad", Type: "dash-folder"},
		{UID: "empty-sub", Title: "empty", Type: "dash-folder", FolderUID: "dry-run-Scratchpad"},
	}
	client.dashboards = []sdk.FoundBoard{
		{UID: "old-dash", Title: "Old Dashboard", FolderUID: "dry-run-Scratchpad"},
	}
	client.dashboardsByUID["old-dash"] = sdk.BoardProperties{
		Created: now.Add(-8 * 24 * time.Hour),
	}

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, true, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.createdFolders) != 0 {
		t.Fatalf("expected no folder creation in dry-run, got %v", client.createdFolders)
	}
	if len(client.updatedPermissions) != 0 {
		t.Fatalf("expected no permission updates in dry-run, got %v", client.updatedPermissions)
	}
	if len(client.deletedDashboardUIDs) != 0 {
		t.Fatalf("expected no dashboard deletions in dry-run, got %v", client.deletedDashboardUIDs)
	}
	if len(client.deletedFolderUIDs) != 0 {
		t.Fatalf("expected no folder deletions in dry-run, got %v", client.deletedFolderUIDs)
	}
}

func TestSyncScratchFolders_IgnoresDashboardsInOtherFolders(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "scratch-uid"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	client.dashboards = []sdk.FoundBoard{
		{UID: "other-dash", Title: "Other Dashboard", FolderUID: "other-folder"},
	}

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.deletedDashboardUIDs) != 0 {
		t.Fatalf("expected no deletions, got %v", client.deletedDashboardUIDs)
	}
}

func TestSyncScratchFolders_DashboardDeleteErrorIsNonFatal(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "scratch-uid"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	client.dashboards = []sdk.FoundBoard{
		{UID: "old-dash", Title: "Old Dashboard", FolderUID: "scratch-uid"},
	}
	client.dashboardsByUID["old-dash"] = sdk.BoardProperties{
		Created: now.Add(-8 * 24 * time.Hour),
	}
	client.deleteDashboardErr = fmt.Errorf("delete failed")

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("expected no error (non-fatal), got %v", err)
	}
}

func TestSyncScratchFolders_MetadataErrorIsNonFatal(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "scratch-uid"}}
	client.dashboards = []sdk.FoundBoard{
		{UID: "bad-dash", Title: "Bad Dashboard", FolderUID: "scratch-uid"},
	}
	client.getDashboardErr = fmt.Errorf("API error")

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("expected no error (non-fatal), got %v", err)
	}
}

func TestSyncScratchFolders_FolderDeleteErrorIsNonFatal(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "scratch-uid"}}
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	client.searchFolders = []sdk.FoundBoard{
		{UID: "scratch-uid", Title: "Scratchpad", Type: "dash-folder"},
		{UID: "empty-sub", Title: "empty", Type: "dash-folder", FolderUID: "scratch-uid"},
	}
	client.dashboards = nil
	client.deleteFolderErr = fmt.Errorf("folder delete failed")

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("expected no error (non-fatal), got %v", err)
	}
}

func TestSyncScratchFolders_PermissionErrorIsFatal(t *testing.T) {
	client := newMockScratchClient()
	client.folders = []sdk.Folder{{Title: "Scratchpad", UID: "scratch-uid"}}
	client.updatePermsErr = fmt.Errorf("forbidden")
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err == nil {
		t.Fatal("expected error on permission update failure, got nil")
	}
}

func TestSyncScratchFolders_ErrorOnInvalidMaxAge(t *testing.T) {
	client := newMockScratchClient()
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "not-a-duration"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err == nil {
		t.Fatal("expected error for invalid maxAge, got nil")
	}
}

func TestSyncScratchFolders_CreateFolderErrorIsFatal(t *testing.T) {
	client := newMockScratchClient()
	client.createFolderErr = fmt.Errorf("permission denied")
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	folders := []config.ScratchFolder{{Name: "Scratchpad", MaxAgeRaw: "168h"}}
	err := syncScratchFolders(context.Background(), client, folders, false, func() time.Time { return now })
	if err == nil {
		t.Fatal("expected error on folder creation failure, got nil")
	}
}

func TestSyncScratchFolders_NoScratchFolders(t *testing.T) {
	client := newMockScratchClient()
	now := time.Date(2025, 7, 28, 12, 0, 0, 0, time.UTC)

	err := syncScratchFolders(context.Background(), client, nil, false, func() time.Time { return now })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCollectScratchFolderUIDs(t *testing.T) {
	allFolders := []sdk.FoundBoard{
		{UID: "root", Title: "Root", Type: "dash-folder"},
		{UID: "child-1", Title: "Child 1", Type: "dash-folder", FolderUID: "root"},
		{UID: "child-2", Title: "Child 2", Type: "dash-folder", FolderUID: "root"},
		{UID: "grandchild", Title: "Grandchild", Type: "dash-folder", FolderUID: "child-1"},
		{UID: "unrelated", Title: "Unrelated", Type: "dash-folder", FolderUID: "other-root"},
	}

	uids := collectScratchFolderUIDs("root", allFolders)

	expected := map[string]bool{"root": true, "child-1": true, "child-2": true, "grandchild": true}
	if len(uids) != len(expected) {
		t.Fatalf("expected %d UIDs, got %d: %v", len(expected), len(uids), uids)
	}
	for uid := range expected {
		if !uids[uid] {
			t.Errorf("expected UID %q in set", uid)
		}
	}
	if uids["unrelated"] {
		t.Error("unrelated folder should not be in set")
	}
}
