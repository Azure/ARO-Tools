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
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Azure/ARO-Tools/tools/grafanactl/internal/grafana"
)

func TestNewFolderCommandSubcommands(t *testing.T) {
	cmd, err := NewFolderCommand("main")
	require.NoError(t, err)

	got := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		got = append(got, sub.Name())
	}
	require.ElementsMatch(t, []string{"create", "get", "list", "delete"}, got)
}

func TestFolderSubcommandRequiredFlags(t *testing.T) {
	for _, tc := range []struct {
		name          string
		subcommand    string
		wantRequired  []string
		wantAvailable []string
	}{
		{
			name:          "create requires uid and title",
			subcommand:    "create",
			wantRequired:  []string{"uid", "title"},
			wantAvailable: []string{"uid", "title", "dry-run"},
		},
		{
			name:          "get requires uid",
			subcommand:    "get",
			wantRequired:  []string{"uid"},
			wantAvailable: []string{"uid"},
		},
		{
			name:          "delete requires uid",
			subcommand:    "delete",
			wantRequired:  []string{"uid"},
			wantAvailable: []string{"uid", "dry-run"},
		},
		{
			name:          "list requires nothing and offers prefix",
			subcommand:    "list",
			wantAvailable: []string{"prefix"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := NewFolderCommand("main")
			require.NoError(t, err)

			sub := findSubcommand(t, cmd, tc.subcommand)

			for _, name := range tc.wantAvailable {
				require.NotNil(t, sub.Flags().Lookup(name), "flag --%s must exist", name)
			}
			for _, name := range tc.wantRequired {
				flag := sub.Flags().Lookup(name)
				require.NotNil(t, flag, "flag --%s must exist", name)
				require.Contains(t, flag.Annotations[cobra.BashCompOneRequiredFlag], "true",
					"flag --%s must be required", name)
			}
		})
	}
}

// The subcommands that address a single folder must not expose --prefix, and
// the one that lists folders must not expose --uid, so that a flag cannot be
// passed where it would be silently ignored.
func TestFolderSubcommandFlagsAreScoped(t *testing.T) {
	cmd, err := NewFolderCommand("main")
	require.NoError(t, err)

	for _, name := range []string{"create", "get", "delete"} {
		require.Nil(t, findSubcommand(t, cmd, name).Flags().Lookup("prefix"),
			"%s must not accept --prefix", name)
	}

	list := findSubcommand(t, cmd, "list")
	require.Nil(t, list.Flags().Lookup("uid"), "list must not accept --uid")
	require.Nil(t, list.Flags().Lookup("title"), "list must not accept --title")
}

// Each subcommand binds its own options, so a flag given to one must not leak
// into another.
func TestFolderSubcommandsHaveIndependentOptions(t *testing.T) {
	cmd, err := NewFolderCommand("main")
	require.NoError(t, err)

	create := findSubcommand(t, cmd, "create")
	require.NoError(t, create.Flags().Set("uid", "pr-123"))

	get := findSubcommand(t, cmd, "get")
	require.Equal(t, "", get.Flags().Lookup("uid").Value.String())
}

// Every exported Validate* entry point must reject a bad base option before
// it reaches the field checks specific to that operation.
func TestValidateXRejectsBadBaseOptions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		validate func(*RawFolderOptions, context.Context) (*ValidatedFolderOptions, error)
	}{
		{name: "ValidateCreate", validate: (*RawFolderOptions).ValidateCreate},
		{name: "ValidateGet", validate: (*RawFolderOptions).ValidateGet},
		{name: "ValidateDelete", validate: (*RawFolderOptions).ValidateDelete},
		{name: "ValidateGetList", validate: (*RawFolderOptions).ValidateGetList},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := buildCompleteFolderOptions(t)
			opts.UID = "pr-123"
			opts.Title = "PR-123"
			// Neither a Grafana resource ID nor the subscription/group/name triplet.
			opts.SubscriptionID = ""

			_, err := tc.validate(opts, t.Context())
			require.ErrorContains(t, err, "subscription ID, resource group, and grafana name are required")
		})
	}
}

func TestValidateCreateRequiresUIDAndTitle(t *testing.T) {
	for _, tc := range []struct {
		name       string
		uid        string
		title      string
		wantErrSub string
	}{
		{name: "missing uid", uid: "", title: "PR-123", wantErrSub: "folder UID is required"},
		{name: "missing title", uid: "pr-123", title: "", wantErrSub: "folder title is required"},
		{
			name:       "uid too long",
			uid:        strings.Repeat("a", grafana.MaxUIDLength+1),
			title:      "PR-123",
			wantErrSub: "too long",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := buildCompleteFolderOptions(t)
			opts.UID = tc.uid
			opts.Title = tc.title

			_, err := opts.ValidateCreate(t.Context())
			require.ErrorContains(t, err, tc.wantErrSub)
		})
	}
}

func TestValidateCreateAcceptsValidInput(t *testing.T) {
	opts := buildCompleteFolderOptions(t)
	opts.UID = "pr-123"
	opts.Title = "PR-123"

	validated, err := opts.ValidateCreate(t.Context())
	require.NoError(t, err)
	require.Equal(t, "pr-123", validated.UID)
	require.Equal(t, "PR-123", validated.Title)
}

// ValidateGet and ValidateDelete both wrap the private validateUID, so they
// share this table: a missing UID and an overlong one are both rejected
// before Complete builds a Grafana client, same as ValidateCreate.
func TestValidateGetAndValidateDeleteRejectBadUID(t *testing.T) {
	for _, fn := range []struct {
		name     string
		validate func(*RawFolderOptions, context.Context) (*ValidatedFolderOptions, error)
	}{
		{name: "ValidateGet", validate: (*RawFolderOptions).ValidateGet},
		{name: "ValidateDelete", validate: (*RawFolderOptions).ValidateDelete},
	} {
		for _, tc := range []struct {
			name       string
			uid        string
			wantErrSub string
		}{
			{name: "missing uid", uid: "", wantErrSub: "folder UID is required"},
			{name: "uid too long", uid: strings.Repeat("a", grafana.MaxUIDLength+1), wantErrSub: "too long"},
		} {
			t.Run(fn.name+"/"+tc.name, func(t *testing.T) {
				opts := buildCompleteFolderOptions(t)
				opts.UID = tc.uid

				_, err := fn.validate(opts, t.Context())
				require.ErrorContains(t, err, tc.wantErrSub)
			})
		}
	}
}

func TestValidateGetAndValidateDeleteAcceptAWellFormedUID(t *testing.T) {
	for _, tc := range []struct {
		name     string
		validate func(*RawFolderOptions, context.Context) (*ValidatedFolderOptions, error)
	}{
		{name: "ValidateGet", validate: (*RawFolderOptions).ValidateGet},
		{name: "ValidateDelete", validate: (*RawFolderOptions).ValidateDelete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := buildCompleteFolderOptions(t)
			opts.UID = "pr-123"

			validated, err := tc.validate(opts, t.Context())
			require.NoError(t, err)
			require.Equal(t, "pr-123", validated.UID)
		})
	}
}

// GetList has no required fields: an empty UID, Title, and Prefix are all valid.
func TestValidateGetListHasNoRequiredFields(t *testing.T) {
	opts := buildCompleteFolderOptions(t)

	_, err := opts.ValidateGetList(t.Context())
	require.NoError(t, err)
}

func buildCompleteFolderOptions(t *testing.T) *RawFolderOptions {
	t.Helper()

	opts := BuildDefaultFolderOptions()
	opts.SubscriptionID = "00000000-0000-0000-0000-000000000000"
	opts.ResourceGroup = "global"
	opts.GrafanaName = "arohcp-dev"
	return opts
}

// A caller outside this module cannot import the internal Grafana package, so
// ErrNotFound must be the same value the client returns.
func TestErrNotFoundMatchesClientSentinel(t *testing.T) {
	wrapped := fmt.Errorf("failed to get folder %q: %w", "pr-123", grafana.ErrFolderNotFound)
	require.ErrorIs(t, wrapped, ErrNotFound)
}

func TestBuildFolderURL(t *testing.T) {
	require.Equal(t,
		"https://g.example.com/dashboards/f/pr-123",
		BuildFolderURL("https://g.example.com", "pr-123"))
}

func findSubcommand(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()

	for _, sub := range parent.Commands() {
		if sub.Name() == name {
			return sub
		}
	}

	t.Fatalf("subcommand %q not found", name)
	return nil
}
