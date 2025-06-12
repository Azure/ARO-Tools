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

package config

import (
	"strings"
)

// Configuration is the top-level container for all values for all services. See an example at: https://github.com/Azure/ARO-HCP/blob/main/config/config.yaml
type Configuration map[string]any

func (v Configuration) GetByPath(path string) (any, bool) {
	keys := strings.Split(path, ".")
	var current any = v

	for _, key := range keys {
		if m, ok := current.(Configuration); ok {
			current, ok = m[key]
			if !ok {
				return nil, false
			}
		} else {
			return nil, false
		}
	}

	return current, true
}

// configurationOverrides is the internal representation for config stored on disk - we do not export it as we
// require that users pre-process it first, which the ConfigProvider.GetResolver() will do for them.
type configurationOverrides struct {
	Schema   string        `json:"$schema"`
	Defaults Configuration `json:"defaults"`
	// key is the cloud alias
	Overrides map[string]*struct {
		Defaults Configuration `json:"defaults"`
		// key is the deploy env
		Overrides map[string]*struct {
			Defaults Configuration `json:"defaults"`
			// key is the region name
			Overrides map[string]Configuration `json:"regions"`
		} `json:"environments"`
	} `json:"clouds"`
}
