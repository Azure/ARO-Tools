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

package azure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
)

func TestNewKustoClusterQueryRequestScopesSubscription(t *testing.T) {
	format := armresourcegraph.ResultFormatObjectArray
	request, err := newKustoClusterQueryRequest("subscription-id", "int-one", format, nil)
	require.NoError(t, err)

	require.Len(t, request.Subscriptions, 1)
	require.NotNil(t, request.Subscriptions[0])
	assert.Equal(t, "subscription-id", *request.Subscriptions[0])
	require.NotNil(t, request.Query)
	assert.Contains(t, *request.Query, "tostring(tags['aroHCPEnvironment']) =~ 'int-one'")
	assert.NotContains(t, *request.Query, "provisioningState) =~ 'Succeeded'")
	assert.Contains(t, *request.Query, "provisioningState = tostring(properties.provisioningState)")
	assert.Contains(t, *request.Query, "| order by id asc")
}

func TestNewKustoClusterQueryRequestRejectsUnsafeEnvironment(t *testing.T) {
	format := armresourcegraph.ResultFormatObjectArray
	_, err := newKustoClusterQueryRequest("subscription-id", "int' | take 1", format, nil)
	assert.ErrorContains(t, err, "only ASCII letters, numbers, and hyphens")
}
