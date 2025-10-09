package types

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

// Configuration is the top-level container for all values for all services. See an example at: https://github.com/Azure/ARO-HCP/blob/main/config/config.yaml
type Configuration map[string]any

type MissingKeyError struct {
	Path string
	Key  string
}

func (e MissingKeyError) Error() string {
	return fmt.Sprintf("configuration%s: key %s not found", e.Path, e.Key)
}

func (v Configuration) GetByPath(path string) (any, error) {
	keys := strings.Split(path, ".")
	var current any = map[string]any(v)
	var currentPath string

	for _, key := range keys {
		if m, ok := current.(map[string]any); ok {
			current, ok = m[key]
			if !ok {
				return nil, &MissingKeyError{Path: currentPath, Key: key}
			}
		} else {
			return nil, fmt.Errorf("configuration%s: expected nested map, found %T; cannot index with %s", currentPath, current, key)
		}
		currentPath += "[" + key + "]"
	}

	return current, nil
}

// MergeConfiguration returns a new configuration holding keys from base, unless they have been overridden.
// This function does not mutate its inputs, but returns a `map[string]any` instead of `types.Configuration`, so
// if your consumer is sensitive to the distinction, remember to cast the output.
func MergeConfiguration(base, override Configuration) map[string]any {
	if base == nil {
		base = Configuration{}
	}
	if override == nil {
		override = Configuration{}
	}
	output := make(Configuration, len(base))
	for k, v := range base {
		output[k] = v
	}
	for k, newValue := range override {
		if baseValue, exists := output[k]; exists {
			srcMap, srcMapOk := newValue.(map[string]any)
			dstMap, dstMapOk := baseValue.(map[string]any)
			if srcMapOk && dstMapOk {
				newValue = MergeConfiguration(dstMap, srcMap)
			}
		}
		output[k] = newValue
	}

	return output
}

// TruncateConfiguration returns a new configuration with specified paths excluded from the base configuration.
// Paths use dot notation (e.g., "database.host", "api.endpoints.users").
// Returns an error if config is nil, no paths are provided, or if any path is invalid.
func TruncateConfiguration(config Configuration, paths ...string) (map[string]any, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no paths provided for truncation")
	}

	// Validate paths
	for _, path := range paths {
		if err := validateTruncatePath(path); err != nil {
			return nil, fmt.Errorf("invalid truncate path %q: %w", path, err)
		}
	}

	result := truncateConfigurationRecursive(map[string]any(config), sets.New(paths...), "")
	return result, nil
}

// validPathRegex matches valid dot notation paths (one or more segments separated by dots)
var validPathRegex = regexp.MustCompile(`^[^.]+(\.[^.]+)*$`)

// validateTruncatePath validates that a path is in proper dot notation format
func validateTruncatePath(path string) error {
	if !validPathRegex.MatchString(path) {
		return fmt.Errorf("path must be in dot notation format (e.g., 'key' or 'parent.child')")
	}
	return nil
}

// truncateConfigurationRecursive recursively copies the configuration while excluding specified paths
func truncateConfigurationRecursive(current map[string]any, truncatePaths sets.Set[string], currentPath string) map[string]any {
	if current == nil {
		return nil
	}

	output := make(map[string]any)

	for key, value := range current {
		var fullPath string
		if currentPath == "" {
			fullPath = key
		} else {
			fullPath = currentPath + "." + key
		}

		if truncatePaths.Has(fullPath) {
			continue
		}

		if nestedMap, ok := value.(map[string]any); ok {
			result := truncateConfigurationRecursive(nestedMap, truncatePaths, fullPath)
			output[key] = result
		} else {
			// Not a nested map, copy the value as-is
			output[key] = value
		}
	}

	return output
}
