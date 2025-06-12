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
	"os"
	"path/filepath"
	"reflect"
	"text/template"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"sigs.k8s.io/yaml"
)

// ConfigReplacements holds replacement values
type ConfigReplacements struct {
	RegionReplacement      string
	RegionShortReplacement string
	StampReplacement       string
	CloudReplacement       string
	EnvironmentReplacement string

	Ev2Config map[string]interface{}
}

// AsMap returns a map[string]interface{} representation of this ConfigReplacement instance
func (c *ConfigReplacements) AsMap() map[string]interface{} {
	m := map[string]interface{}{
		"ctx": map[string]interface{}{
			"region":      c.RegionReplacement,
			"regionShort": c.RegionShortReplacement,
			"stamp":       c.StampReplacement,
			"cloud":       c.CloudReplacement,
			"environment": c.EnvironmentReplacement,
		},
		"ev2": c.Ev2Config,
	}
	return m
}

// ConfigProvider provides service configuration using a base configuration file.
type ConfigProvider interface {
	// AllContexts determines all the clouds, environments, and regions that this provider has explicit records for.
	AllContexts() map[string]map[string][]string
	// GetResolver consumes the configuration replacements to create a configuration resolver.
	// The cloud and environment provided in the replacements must be literal values, used to
	// constrain the resolver further and ensure that configurations it resolves are correct.
	GetResolver(configReplacements *ConfigReplacements) (ConfigResolver, error)
}

// ConfigResolver resolves service configuration for a specific environment and cloud using a processed configuration file.
type ConfigResolver interface {
	// ValidateSchema validates a fully resolved configuration created by this provider.
	ValidateSchema(config Configuration) error
	// GetConfiguration resolves the configuration for the cloud and environment.
	GetConfiguration() (Configuration, error)
	// GetRegionConfiguration resolves the configuration for a region in the cloud and environment.
	GetRegionConfiguration(region string) (Configuration, error)
	// GetRegionOverrides fetches the overrides specific to a region, if any exist.
	GetRegionOverrides(region string) (Configuration, error)
}

func NewConfigProvider(config string) (ConfigProvider, error) {
	cp := configProvider{
		path: config,
	}

	raw, err := os.ReadFile(config)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(raw, &cp.raw); err != nil {
		return nil, err
	}

	return &cp, nil
}

type configProvider struct {
	// schema can be a relative path to this file, so we need to keep track of it
	path string
	raw  configurationOverrides
}

// AllContexts returns all clouds, environments and regions in the configuration.
func (cp *configProvider) AllContexts() map[string]map[string][]string {
	contexts := map[string]map[string][]string{}
	for cloud, cloudCfg := range cp.raw.Overrides {
		contexts[cloud] = map[string][]string{}
		for environment, envCfg := range cloudCfg.Overrides {
			contexts[cloud][environment] = []string{}
			for region := range envCfg.Overrides {
				contexts[cloud][environment] = append(contexts[cloud][environment], region)
			}
		}
	}
	return contexts
}

func (cp *configProvider) GetResolver(configReplacements *ConfigReplacements) (ConfigResolver, error) {
	for description, value := range map[string]*string{
		"cloud":       &configReplacements.CloudReplacement,
		"environment": &configReplacements.EnvironmentReplacement,
	} {
		if value == nil || *value == "" {
			return nil, fmt.Errorf("%q override is required", description)
		}
	}

	// TODO validate that field names are unique regardless of casing
	// parse, execute and unmarshal the config file as a template to generate the final config file
	encoded, err := yaml.Marshal(cp.raw)
	if err != nil {
		return nil, err
	}

	rawContent, err := PreprocessContent(encoded, configReplacements.AsMap())
	if err != nil {
		return nil, err
	}

	currentVariableOverrides := configurationOverrides{}
	if err := yaml.Unmarshal(rawContent, &currentVariableOverrides); err != nil {
		return nil, err
	}
	return &configResolver{
		cloud:       configReplacements.CloudReplacement,
		environment: configReplacements.EnvironmentReplacement,
		cfg:         currentVariableOverrides,
		path:        cp.path,
	}, nil
}

type configResolver struct {
	cloud, environment string
	cfg                configurationOverrides
	path               string
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
	if base == nil {
		base = Configuration{}
	}
	if override == nil {
		override = Configuration{}
	}
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

func (cr *configResolver) ValidateSchema(config Configuration) error {
	loader := jsonschema.SchemeURLLoader{
		"file": jsonschema.FileLoader{},
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(loader)
	path := cr.cfg.Schema
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(filepath.Join(filepath.Dir(cr.path), path))
		if err != nil {
			return fmt.Errorf("failed to create absolute path to schema %q: %w", path, err)
		}
		path = absPath
	}
	sch, err := c.Compile(path)
	if err != nil {
		return fmt.Errorf("failed to compile schema: %v", err)
	}

	err = sch.Validate(convertToInterface(config))
	if err != nil {
		return fmt.Errorf("failed to validate schema: %v", err)
	}
	return nil
}

// GetRegionConfiguration merges values to resolve the configuration for a region.
func (cr *configResolver) GetRegionConfiguration(region string) (Configuration, error) {
	cfg := cr.cfg.Defaults
	cloudCfg, hasCloud := cr.cfg.Overrides[cr.cloud]
	if !hasCloud {
		return nil, fmt.Errorf("the cloud %s is not found in the config", cr.cloud)
	}
	MergeConfiguration(cfg, cloudCfg.Defaults)
	envCfg, hasEnv := cloudCfg.Overrides[cr.environment]
	if !hasEnv {
		return nil, fmt.Errorf("the deployment env %s is not found under cloud %s", cr.environment, cr.cloud)
	}
	MergeConfiguration(cfg, envCfg.Defaults)
	regionCfg, hasRegion := envCfg.Overrides[region]
	if !hasRegion {
		// a missing region just means we use default values
		regionCfg = Configuration{}
	}
	MergeConfiguration(cfg, regionCfg)
	return cfg, nil
}

// GetConfiguration merges values to resolve the configuration for this cloud and environment.
func (cr *configResolver) GetConfiguration() (Configuration, error) {
	cfg := cr.cfg.Defaults
	cloudCfg, hasCloud := cr.cfg.Overrides[cr.cloud]
	if !hasCloud {
		return nil, fmt.Errorf("the cloud %s is not found in the config", cr.cloud)
	}
	MergeConfiguration(cfg, cloudCfg.Defaults)
	envCfg, hasEnv := cloudCfg.Overrides[cr.environment]
	if !hasEnv {
		return nil, fmt.Errorf("the deployment env %s is not found under cloud %s", cr.environment, cr.cloud)
	}
	MergeConfiguration(cfg, envCfg.Defaults)

	return cfg, nil
}

// GetRegionOverrides resolves the overrides for a region.
func (cr *configResolver) GetRegionOverrides(region string) (Configuration, error) {
	cloudCfg, hasCloud := cr.cfg.Overrides[cr.cloud]
	if !hasCloud {
		return nil, fmt.Errorf("the cloud %s is not found in the config", cr.cloud)
	}
	envCfg, hasEnv := cloudCfg.Overrides[cr.environment]
	if !hasEnv {
		return nil, fmt.Errorf("the deployment env %s is not found under cloud %s", cr.environment, cr.cloud)
	}
	regionCfg, hasRegion := envCfg.Overrides[region]
	if !hasRegion {
		// a missing region just means we use default values
		regionCfg = Configuration{}
	}
	return regionCfg, nil
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
		return nil, fmt.Errorf("failed to preprocess content %s: %w", templateFilePath, err)
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
