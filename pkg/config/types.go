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
	"log/slog"
	"strings"
)

type configProviderImpl struct {
	config string
	schema string
}

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

func NewConfigurationOverrides() ConfigurationOverrides {
	return &configurationOverrides{}
}

type ConfigurationOverrides interface {
	GetDefaults() Configuration
	GetCloudOverrides(cloud string) Configuration
	GetDeployEnvOverrides(cloud, deployEnv string) Configuration
	ResolveEnvironment(cloud, environment string) Configuration
	GetRegionOverrides(cloud, deployEnv, region string) Configuration
	ResolveRegion(cloud, environment, region string) Configuration
	GetRegions(cloud, deployEnv string) []string
	GetSchema() string
	HasSchema() bool
	HasCloud(cloud string) bool
	HasDeployEnv(cloud, deployEnv string) bool
}

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

func (vo *configurationOverrides) GetSchema() string {
	return vo.Schema
}

func (vo *configurationOverrides) HasSchema() bool {
	return vo.Schema != ""
}

func (vo *configurationOverrides) GetDefaults() Configuration {
	return vo.Defaults
}

func (vo *configurationOverrides) HasCloud(cloud string) bool {
	_, ok := vo.Overrides[cloud]
	return ok
}

func (vo *configurationOverrides) GetCloudOverrides(cloud string) Configuration {
	if cloudOverride, ok := vo.Overrides[cloud]; ok {
		return cloudOverride.Defaults
	}
	return Configuration{}
}

func (vo *configurationOverrides) HasDeployEnv(cloud, deployEnv string) bool {
	if cloudOverride, ok := vo.Overrides[cloud]; ok {
		_, ok := cloudOverride.Overrides[deployEnv]
		return ok
	}
	return false
}

func (vo *configurationOverrides) GetDeployEnvOverrides(cloud, deployEnv string) Configuration {
	if cloudOverride, ok := vo.Overrides[cloud]; ok {
		if deployEnvOverride, ok := cloudOverride.Overrides[deployEnv]; ok {
			return deployEnvOverride.Defaults
		}
	}
	return Configuration{}
}

func (vo *configurationOverrides) GetRegions(cloud, deployEnv string) []string {
	deployEnvOverrides, err := vo.getAllDeployEnvRegionOverrides(cloud, deployEnv)
	if err != nil {
		return []string{}
	}
	regions := make([]string, 0, len(deployEnvOverrides))
	for region := range deployEnvOverrides {
		regions = append(regions, region)
	}
	return regions
}

func (vo *configurationOverrides) getAllDeployEnvRegionOverrides(cloud, deployEnv string) (map[string]Configuration, error) {
	if cloudOverride, ok := vo.Overrides[cloud]; ok {
		if deployEnvOverride, ok := cloudOverride.Overrides[deployEnv]; ok {
			return deployEnvOverride.Overrides, nil
		} else {
			return nil, fmt.Errorf("deploy env %s not found under cloud %s in config", deployEnv, cloud)
		}
	}
	return nil, fmt.Errorf("cloud %s not found in config", cloud)
}

func (vo *configurationOverrides) GetRegionOverrides(cloud, deployEnv, region string) Configuration {
	regionOverrides, err := vo.getAllDeployEnvRegionOverrides(cloud, deployEnv)
	if err != nil {
		slog.Warn("Failed to get region overrides", "err", err)
		return Configuration{}
	}
	if regionOverrides, ok := regionOverrides[region]; ok {
		return regionOverrides
	} else {
		slog.Warn("Failed to find region in config", "region", region)
		return Configuration{}
	}
}

func (vo *configurationOverrides) ResolveEnvironment(cloud, environment string) Configuration {
	mergedConfig := Configuration{}
	MergeConfiguration(mergedConfig, vo.GetDefaults())
	MergeConfiguration(mergedConfig, vo.GetCloudOverrides(cloud))
	MergeConfiguration(mergedConfig, vo.GetDeployEnvOverrides(cloud, environment))
	return mergedConfig
}

func (vo *configurationOverrides) ResolveRegion(cloud, environment, region string) Configuration {
	mergedConfig := vo.ResolveEnvironment(cloud, environment)
	MergeConfiguration(mergedConfig, vo.GetRegionOverrides(cloud, environment, region))
	return mergedConfig
}
