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

package entitygroups

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kustoazure "github.com/Azure/ARO-Tools/tools/kustoctl/internal/azure"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
)

func TestBuildEntityGroupKQL(t *testing.T) {
	clusters := []kustoazure.KustoCluster{
		{Name: "hcp-prod-au", Location: "australiaeast", URI: "https://hcp-prod-au.australiaeast.kusto.windows.net"},
		{Name: "hcp-prod-br", Location: "brazilsouth", URI: "https://hcp-prod-br.brazilsouth.kusto.windows.net"},
		{Name: "hcp-prod-uk", Location: "uksouth", URI: "https://hcp-prod-uk.uksouth.kusto.windows.net"},
	}

	result, err := buildEntityGroupKQL("HCPServiceLogsEG", "ServiceLogs", clusters)
	require.NoError(t, err)

	expected := `.create-or-alter entity_group HCPServiceLogsEG (
    cluster('https://hcp-prod-au.australiaeast.kusto.windows.net').database('ServiceLogs'),
    cluster('https://hcp-prod-br.brazilsouth.kusto.windows.net').database('ServiceLogs'),
    cluster('https://hcp-prod-uk.uksouth.kusto.windows.net').database('ServiceLogs')
)`
	assert.Equal(t, expected, result)
}

func TestBuildEntityGroupKQL_SingleCluster(t *testing.T) {
	clusters := []kustoazure.KustoCluster{
		{Name: "hcp-int-uk", Location: "uksouth", URI: "https://hcp-int-uk.uksouth.kusto.windows.net"},
	}

	result, err := buildEntityGroupKQL("HCPServiceLogsEG", "ServiceLogs", clusters)
	require.NoError(t, err)

	expected := `.create-or-alter entity_group HCPServiceLogsEG (
    cluster('https://hcp-int-uk.uksouth.kusto.windows.net').database('ServiceLogs')
)`
	assert.Equal(t, expected, result)
}

func TestBuildEntityGroupKQL_EmptyClusters(t *testing.T) {
	_, err := buildEntityGroupKQL("HCPServiceLogsEG", "ServiceLogs", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no clusters")
}

func TestValidate_EntityGroupNameValidation(t *testing.T) {
	tests := []struct {
		name    string
		eg      string
		wantErr bool
	}{
		{"valid", "HCPServiceLogsEG:ServiceLogs", false},
		{"valid underscore", "My_Group:MyDB", false},
		{"invalid spaces", "foo bar:ServiceLogs", true},
		{"invalid parens", "foo():ServiceLogs", true},
		{"injection attempt", "foo; .drop table:ServiceLogs", true},
		{"empty name", ":ServiceLogs", true},
		{"empty database", "HCPServiceLogsEG:", true},
		{"no colon", "HCPServiceLogsEGServiceLogs", true},
		{"starts with number", "1Group:DB", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &RawSyncOptions{EntityGroups: []string{tt.eg}}
			_, err := opts.Validate(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate_DuplicateEntityGroupName(t *testing.T) {
	opts := &RawSyncOptions{
		EntityGroups: []string{"EG1:DB1", "EG1:DB2"},
	}
	_, err := opts.Validate(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestValidate_EmptyEntityGroups(t *testing.T) {
	opts := &RawSyncOptions{}
	_, err := opts.Validate(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one")
}

func TestValidate_CloudConfigDoesNotMutateGlobal(t *testing.T) {
	// Capture original public cloud services map length
	originalLen := len(cloud.AzurePublic.Services)

	opts := &RawSyncOptions{
		EntityGroups: []string{"EG1:DB1"},
		ARMEndpoint:  "https://custom.arm.endpoint",
		AADAuthority: "https://custom.aad.authority",
	}
	validated, err := opts.Validate(context.Background())
	require.NoError(t, err)

	// Custom config should have the custom endpoints
	assert.Equal(t, "https://custom.aad.authority", validated.cloudConfig.ActiveDirectoryAuthorityHost)

	// Global should NOT be mutated
	assert.Equal(t, originalLen, len(cloud.AzurePublic.Services), "cloud.AzurePublic.Services was mutated")
	assert.Equal(t, "https://login.microsoftonline.com/", cloud.AzurePublic.ActiveDirectoryAuthorityHost, "cloud.AzurePublic.ActiveDirectoryAuthorityHost was mutated")
}

func TestValidate_CloudConfigBothOrNeither(t *testing.T) {
	opts := &RawSyncOptions{
		EntityGroups: []string{"EG1:DB1"},
		ARMEndpoint:  "https://arm.endpoint",
	}
	_, err := opts.Validate(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "both be set")
}

func TestValidate_DatabaseNameRejection(t *testing.T) {
	tests := []struct {
		name    string
		eg      string
		wantErr bool
	}{
		{"valid db", "EG:ServiceLogs", false},
		{"valid db with hyphen", "EG:Hosted-Control-Plane", false},
		{"db with quotes", "EG:foo'bar", true},
		{"db with semicolon", "EG:foo;drop", true},
		{"db with spaces", "EG:foo bar", true},
		{"db with parens", "EG:foo()", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &RawSyncOptions{EntityGroups: []string{tt.eg}}
			_, err := opts.Validate(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate_EnvironmentValidation(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		wantErr bool
	}{
		{"empty is allowed (legacy cross-env)", "", false},
		{"int", "int", false},
		{"stg", "stg", false},
		{"prod", "prod", false},
		{"with hyphen and underscore", "us-gov_1", false},
		{"with quote", "int'", true},
		{"with space", "in t", true},
		{"with semicolon", "int;drop", true},
		{"with bracket", "tags['x']", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &RawSyncOptions{
				EntityGroups: []string{"EG:ServiceLogs"},
				Environment:  tt.env,
			}
			_, err := opts.Validate(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSelectClustersForEnvironment(t *testing.T) {
	int1 := kustoazure.KustoCluster{Name: "hcp-int-a", Location: "eastus", URI: "https://hcp-int-a.eastus.kusto.windows.net", Environment: "int"}
	int2 := kustoazure.KustoCluster{Name: "hcp-int-b", Location: "westus", URI: "https://hcp-int-b.westus.kusto.windows.net", Environment: "INT"}
	stg1 := kustoazure.KustoCluster{Name: "hcp-stg-a", Location: "eastus", URI: "https://hcp-stg-a.eastus.kusto.windows.net", Environment: "stg"}

	t.Run("empty environment returns all unchanged", func(t *testing.T) {
		untagged := kustoazure.KustoCluster{Name: "hcp-untagged", Location: "eastus", URI: "https://hcp-untagged.eastus.kusto.windows.net"}
		all := []kustoazure.KustoCluster{int1, stg1, untagged}
		got, err := selectClustersForEnvironment(all, "")
		assert.NoError(t, err)
		assert.Equal(t, all, got)
	})

	t.Run("filters case-insensitively to the target environment", func(t *testing.T) {
		got, err := selectClustersForEnvironment([]kustoazure.KustoCluster{int1, int2, stg1}, "int")
		assert.NoError(t, err)
		assert.Equal(t, []kustoazure.KustoCluster{int1, int2}, got)
	})

	t.Run("fails closed when a cluster is missing the aroHCPEnvironment tag", func(t *testing.T) {
		untagged := kustoazure.KustoCluster{Name: "hcp-new", Location: "eastus", URI: "https://hcp-new.eastus.kusto.windows.net"}
		_, err := selectClustersForEnvironment([]kustoazure.KustoCluster{int1, untagged}, "int")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "hcp-new")
		assert.Contains(t, err.Error(), "aroHCPEnvironment")
	})

	t.Run("fails closed when a cluster has an invalid tag value", func(t *testing.T) {
		bad := kustoazure.KustoCluster{Name: "hcp-bad", Location: "eastus", URI: "https://hcp-bad.eastus.kusto.windows.net", Environment: "int prod"}
		_, err := selectClustersForEnvironment([]kustoazure.KustoCluster{int1, bad}, "int")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "hcp-bad")
	})

	t.Run("returns empty when fully tagged but none match the target", func(t *testing.T) {
		got, err := selectClustersForEnvironment([]kustoazure.KustoCluster{stg1}, "int")
		assert.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"bare hostname", "management.azure.com", "https://management.azure.com", false},
		{"full https URL", "https://management.azure.com", "https://management.azure.com", false},
		{"http rejected", "http://insecure.endpoint", "", true},
		{"strips path", "https://management.azure.com/foo", "https://management.azure.com", false},
		{"strips query", "https://management.azure.com?bar=baz", "https://management.azure.com", false},
		{"empty", "", "", true},
		{"whitespace only", "   ", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeEndpoint(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
