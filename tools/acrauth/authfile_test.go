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

package acrauth

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func expectedAuth(t *testing.T, username, password string) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

func readBack(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(raw, &document))
	return document
}

func TestUpsertCredentialPreservesOtherRegistries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	existing := `{
  "auths": {
    "registry.redhat.io": { "auth": "cmVkaGF0OnNlY3JldA==", "email": "noreply@redhat.com" }
  }
}`
	require.NoError(t, os.WriteFile(path, []byte(existing), 0o600))

	require.NoError(t, UpsertCredential(path, "arohcpocpstg.azurecr.io", NullGUIDUsername, "token"))

	auths := readBack(t, path)["auths"].(map[string]any)
	require.Len(t, auths, 2)

	upstream := auths["registry.redhat.io"].(map[string]any)
	require.Equal(t, "cmVkaGF0OnNlY3JldA==", upstream["auth"])
	require.Equal(t, "noreply@redhat.com", upstream["email"], "fields we do not model must survive a rewrite")

	added := auths["arohcpocpstg.azurecr.io"].(map[string]any)
	require.Equal(t, expectedAuth(t, NullGUIDUsername, "token"), added["auth"])
}

func TestUpsertCredentialReplacesSameRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, UpsertCredential(path, "acr.azurecr.io", NullGUIDUsername, "first"))
	require.NoError(t, UpsertCredential(path, "acr.azurecr.io", NullGUIDUsername, "second"))

	auths := readBack(t, path)["auths"].(map[string]any)
	require.Len(t, auths, 1)
	entry := auths["acr.azurecr.io"].(map[string]any)
	require.Equal(t, expectedAuth(t, NullGUIDUsername, "second"), entry["auth"])
}

func TestUpsertCredentialCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "containers", "auth.json")

	require.NoError(t, UpsertCredential(path, "acr.azurecr.io", NullGUIDUsername, "token"))

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, authFileMode, info.Mode().Perm(), "credential file must not be world-readable")

	auths := readBack(t, path)["auths"].(map[string]any)
	require.Contains(t, auths, "acr.azurecr.io")
}

func TestUpsertCredentialRejectsEmptyRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.Error(t, UpsertCredential(path, "", NullGUIDUsername, "token"))
}
