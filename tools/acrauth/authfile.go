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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// authFileMode is the mode os.CreateTemp gives the temporary file, which survives the rename.
const authFileMode fs.FileMode = 0o600

// UpsertCredential adds or replaces the entry for one registry in a container auth file,
// leaving every other registry and any fields we don't model untouched.
func UpsertCredential(path, registry, username, password string) error {
	if registry == "" {
		return errors.New("registry must not be empty")
	}

	document, err := readAuthFile(path)
	if err != nil {
		return err
	}

	auths, ok := document["auths"].(map[string]any)
	if !ok {
		auths = map[string]any{}
	}

	entry, ok := auths[registry].(map[string]any)
	if !ok {
		entry = map[string]any{}
	}
	entry["auth"] = base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

	auths[registry] = entry
	document["auths"] = auths

	return writeAuthFile(path, document)
}

func readAuthFile(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read auth file %q: %w", path, err)
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}

	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("failed to parse auth file %q: %w", path, err)
	}
	if document == nil {
		document = map[string]any{}
	}
	return document, nil
}

// writeAuthFile writes through a temporary file in the same directory so a failure part-way
// through cannot leave the tool holding a truncated credential file.
func writeAuthFile(path string, document map[string]any) error {
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize auth file: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dir, err)
	}

	// CreateTemp opens with mode 0600, so the credential is never briefly world-readable.
	tmp, err := os.CreateTemp(dir, ".auth-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temporary auth file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write temporary auth file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temporary auth file: %w", err)
	}

	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("failed to replace auth file %q: %w", path, err)
	}
	return nil
}
