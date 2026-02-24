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
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// ObservabilityConfig represents the full observability configuration file
type ObservabilityConfig struct {
	GrafanaDashboards GrafanaDashboardsConfig `json:"grafana-dashboards"`
}

// GrafanaDashboardsConfig represents the grafana-dashboards section
type GrafanaDashboardsConfig struct {
	AzureManagedFolders []string          `json:"azureManagedFolders"`
	DashboardFolders    []DashboardFolder `json:"dashboardFolders"`
}

// DashboardFolder represents a folder containing dashboards to sync
type DashboardFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// LoadFromFile reads and parses the observability config from a file
func LoadFromFile(path string) (*ObservabilityConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg ObservabilityConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}
