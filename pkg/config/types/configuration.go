package types

import (
	"reflect"
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
