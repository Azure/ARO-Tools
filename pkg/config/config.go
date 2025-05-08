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
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"text/template"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// DefaultConfigReplacements has no replacements configured
func DefaultConfigReplacements() *ConfigReplacements {
	return &ConfigReplacements{}
}

// ConfigReplacements holds replacement values
type ConfigReplacements struct {
	RegionReplacement      string
	RegionShortReplacement string
	StampReplacement       string
	CloudReplacement       string
	EnvironmentReplacement string
}

// AsMap returns a map[string]interface{} representation of this ConfigReplacement instance
func (c *ConfigReplacements) AsMap() map[string]interface{} {
	return map[string]interface{}{
		"ctx": map[string]interface{}{
			"region":      c.RegionReplacement,
			"regionShort": c.RegionShortReplacement,
			"stamp":       c.StampReplacement,
			"cloud":       c.CloudReplacement,
			"environment": c.EnvironmentReplacement,
		},
	}
}

// ConfigProvider resolves service configuration for specific environments and clouds using a base configuration file.
type ConfigProvider interface {
	Validate(cloud, deployEnv string) error
	GetDeployEnvRegionConfiguration(cloud, deployEnv, region string, configReplacements *ConfigReplacements) (Configuration, error)
	GetDeployEnvConfiguration(cloud, deployEnv string, configReplacements *ConfigReplacements) (Configuration, error)
	GetRegions(cloud, deployEnv string) ([]string, error)
	GetRegionOverrides(cloud, deployEnv, region string, configReplacements *ConfigReplacements) (Configuration, error)
}

func NewConfigProvider(config string) ConfigProvider {
	return &configProviderImpl{
		config: config,
	}
}

// InterfaceToConfiguration, pass in an interface of map[string]any and get (Configuration, true) back
// This is also converting nested maps, making it easier to iterate over the configuration.
// If type does not match, second return value will be false
func InterfaceToConfiguration(i interface{}) (Configuration, bool) {
	// Helper, that reduces need for reflection calls, i.e. MapIndex
	// from: https://github.com/peterbourgon/mergemap/blob/master/mergemap.go
	value := reflect.ValueOf(i)
	if value.Kind() == reflect.Map {
		m := Configuration{}
		for _, k := range value.MapKeys() {
			v := value.MapIndex(k).Interface()
			if nestedMap, ok := InterfaceToConfiguration(v); ok {
				m[k.String()] = nestedMap
			} else {
				m[k.String()] = v
			}
		}
		return m, true
	}
	return Configuration{}, false
}

// Merges Configuration, returns merged Configuration
// However the return value is only used for recursive updates on the map
// The actual merged Configuration are updated in the base map
func MergeConfiguration(base, override Configuration) Configuration {
	for k, newValue := range override {
		if baseValue, exists := base[k]; exists {
			srcMap, srcMapOk := InterfaceToConfiguration(newValue)
			dstMap, dstMapOk := InterfaceToConfiguration(baseValue)
			if srcMapOk && dstMapOk {
				newValue = MergeConfiguration(dstMap, srcMap)
			}
		}
		base[k] = newValue
	}

	return base
}

// Needed to convert Configuration to map[string]interface{} for jsonschema validation
// see: https://github.com/santhosh-tekuri/jsonschema/blob/boon/schema.go#L124
func convertToInterface(config Configuration) map[string]any {
	m := map[string]any{}
	for k, v := range config {
		if subMap, ok := v.(Configuration); ok {
			m[k] = convertToInterface(subMap)
		} else {
			m[k] = v
		}
	}
	return m
}

func isUrl(str string) bool {
	u, err := url.Parse(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func (cp *configProviderImpl) loadSchema() (any, error) {
	schemaPath := cp.schema

	var reader io.Reader
	var err error

	if isUrl(schemaPath) {
		resp, err := http.Get(schemaPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema file: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("faild to get schema file, statuscode %v", resp.StatusCode)
		}
		reader = resp.Body
	} else {
		if !filepath.IsAbs(schemaPath) {
			schemaPath = filepath.Join(filepath.Dir(cp.config), schemaPath)
		}
		reader, err = os.Open(schemaPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open schema file: %v", err)
		}
	}

	schema, err := jsonschema.UnmarshalJSON(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal schema: %v", err)
	}

	return schema, nil
}

func (cp *configProviderImpl) validateSchema(config Configuration) error {
	c := jsonschema.NewCompiler()

	schema, err := cp.loadSchema()
	if err != nil {
		return fmt.Errorf("failed to load schema: %v", err)
	}

	err = c.AddResource(cp.schema, schema)
	if err != nil {
		return fmt.Errorf("failed to add schema resource: %v", err)
	}
	sch, err := c.Compile(cp.schema)
	if err != nil {
		return fmt.Errorf("failed to compile schema: %v", err)
	}

	err = sch.Validate(convertToInterface(config))
	if err != nil {
		return fmt.Errorf("failed to validate schema: %v", err)
	}
	return nil
}

// GetDeployEnvRegionConfiguration reads, processes, validates and returns the configuration
// It uses GetDeployEnvConfiguration and will in addition merge region overrides
func (cp *configProviderImpl) GetDeployEnvRegionConfiguration(cloud, deployEnv, region string, configReplacements *ConfigReplacements) (Configuration, error) {
	config, err := cp.GetDeployEnvConfiguration(cloud, deployEnv, configReplacements)
	if err != nil {
		return nil, err
	}

	// region overrides
	regionOverrides, err := cp.GetRegionOverrides(cloud, deployEnv, region, configReplacements)
	if err != nil {
		return nil, err
	}
	MergeConfiguration(config, regionOverrides)

	// validate schema
	err = cp.validateSchema(config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// Validate basic validation
func (cp *configProviderImpl) Validate(cloud, deployEnv string) error {
	config, err := cp.loadConfig(DefaultConfigReplacements())
	if err != nil {
		return err
	}
	if ok := config.HasCloud(cloud); !ok {
		return fmt.Errorf("the cloud %s is not found in the config", cloud)
	}

	if ok := config.HasDeployEnv(cloud, deployEnv); !ok {
		return fmt.Errorf("the deployment env %s is not found under cloud %s", deployEnv, cloud)
	}

	if !config.HasSchema() {
		return fmt.Errorf("$schema not found in config")
	}
	return nil
}

// GetDeployEnvConfiguration load and merge the configuration
// todo: this is called in HCP, so it should use schema validation as well.
func (cp *configProviderImpl) GetDeployEnvConfiguration(cloud, deployEnv string, configReplacements *ConfigReplacements) (Configuration, error) {
	config, err := cp.loadConfig(configReplacements)
	if err != nil {
		return nil, err
	}
	err = cp.Validate(cloud, deployEnv)
	if err != nil {
		return nil, err
	}

	mergedConfig := Configuration{}
	MergeConfiguration(mergedConfig, config.GetDefaults())
	MergeConfiguration(mergedConfig, config.GetCloudOverrides(cloud))
	MergeConfiguration(mergedConfig, config.GetDeployEnvOverrides(cloud, deployEnv))

	cp.schema = config.GetSchema()

	return mergedConfig, nil
}

// GetRegions returns the list of configured regions
func (cp *configProviderImpl) GetRegions(cloud, deployEnv string) ([]string, error) {
	config, err := cp.loadConfig(DefaultConfigReplacements())
	if err != nil {
		return nil, err
	}
	err = cp.Validate(cloud, deployEnv)
	if err != nil {
		return nil, err
	}
	regions := config.GetRegions(cloud, deployEnv)
	return regions, nil
}

// GetRegionOverrides retun a configuration where region overrides have been applied
func (cp *configProviderImpl) GetRegionOverrides(cloud, deployEnv, region string, configReplacements *ConfigReplacements) (Configuration, error) {
	config, err := cp.loadConfig(configReplacements)
	if err != nil {
		return nil, err
	}
	return config.GetRegionOverrides(cloud, deployEnv, region), nil
}

func (cp *configProviderImpl) loadConfig(configReplacements *ConfigReplacements) (ConfigurationOverrides, error) {
	// TODO validate that field names are unique regardless of casing
	// parse, execute and unmarshal the config file as a template to generate the final config file
	rawContent, err := PreprocessFile(cp.config, configReplacements.AsMap())
	if err != nil {
		return nil, err
	}

	currentVariableOverrides := NewConfigurationOverrides()
	if err := yaml.Unmarshal(rawContent, currentVariableOverrides); err == nil {
		return currentVariableOverrides, nil
	} else {
		return nil, err
	}
}

// PreprocessFile reads and processes a gotemplate
// The path will be read as is. It parses the file as a template, and executes it with the provided Configuration.
func PreprocessFile(templateFilePath string, vars map[string]any) ([]byte, error) {
	content, err := os.ReadFile(templateFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", templateFilePath, err)
	}
	processedContent, err := PreprocessContent(content, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to preprocess file %s: %w", templateFilePath, err)
	}
	return processedContent, nil
}

// PreprocessContent processes a gotemplate from memory
func PreprocessContent(content []byte, vars map[string]any) ([]byte, error) {
	var tmplBytes bytes.Buffer
	if err := PreprocessContentIntoWriter(content, vars, &tmplBytes); err != nil {
		return nil, err
	}
	return tmplBytes.Bytes(), nil
}

func PreprocessContentIntoWriter(content []byte, vars map[string]any, writer io.Writer) error {
	tmpl, err := template.New("file").Parse(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	if err := tmpl.Option("missingkey=error").Execute(writer, vars); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	return nil
}
