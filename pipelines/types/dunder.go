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

package types

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/Azure/ARO-Tools/config/types"
)

// PlaceholderGenerator returns, for a given dotted-path key and the leaf's
// reflect.Type, a pair of strings used by EV2Mapping:
//
//   - flattenedKey: the value used as a map key in the flat "placeholder ->
//     dotted path" output map. Most generators set this equal to replaceVar.
//   - replaceVar: the string substituted into the deep-copied configuration
//     tree at the leaf's position.
//
// The leaf type is provided so type-aware generators (for example a Bicep
// parameter generator that must wrap non-strings) can branch on Kind. A nil
// reflect.Type means the leaf's runtime type was not recoverable; generators
// must tolerate this.
type PlaceholderGenerator func(key []string, valueType reflect.Type) (flattenedKey string, replaceVar string)

// NewDunderPlaceholders returns a PlaceholderGenerator that produces the
// "dunder" placeholder format used by sdp-pipelines for pipeline.yaml schema
// validation: each leaf is replaced with a string of the form "__<dotted.path>__".
//
// The output format is intentionally consistent with DefaultPlaceholderPattern
// ("^__.+__$"), so EV2Mapping(typedCfg, NewDunderPlaceholders(), nil) produces
// a Configuration that ValidatePipelineSchemaWithOptions(content,
// WithAllowPlaceholders("")) will accept on every non-string scalar field.
//
// Example:
//
//	key := []string{"foo", "bar"}
//	flattenedKey, replaceVar := NewDunderPlaceholders()(key, nil)
//	// flattenedKey and replaceVar will both be "__foo.bar__"
func NewDunderPlaceholders() PlaceholderGenerator {
	return func(key []string, _ reflect.Type) (flattenedKey string, replaceVar string) {
		flattenedKey = fmt.Sprintf("__%s__", strings.Join(key, "."))
		replaceVar = flattenedKey
		return
	}
}

// EV2Mapping walks a nested configuration tree and returns:
//
//  1. a flat map of placeholder -> dotted key path (e.g. "__foo.bar__" -> "foo.bar"),
//  2. a deep copy of the input tree where each scalar leaf has been replaced
//     by the placeholder string produced by placeholderGenerator.
//
// The walker descends into nested map[string]any values. All other values —
// including []any — are treated as scalar leaves and replaced by a single
// placeholder. This is deliberate: per ARO-HCP configuration policy, arrays in
// per-region configuration are not supported (see
// https://github.com/Azure/ARO-HCP/blob/main/docs/configuration.md#limitations
// — "Avoid using arrays in configuration. Instead, represent arrays as a list
// of comma separated values"). Treating slices as opaque scalars keeps the
// dunder output consistent with Ev2's static-manifest-up-front model.
//
// The prefix argument seeds the dotted-path key for the recursion (callers
// typically pass nil).
//
// Example:
//
//	input := types.Configuration{
//	  "ev2": map[string]any{"replicas": 3, "name": "svc"},
//	  "registries": "quay.io,registry.redhat.io", // CSV per the array-policy
//	}
//	flat, dunder := EV2Mapping(input, NewDunderPlaceholders(), nil)
//	// flat == map[string]string{
//	//   "__ev2.replicas__": "ev2.replicas",
//	//   "__ev2.name__":     "ev2.name",
//	//   "__registries__":   "registries",
//	// }
//	// dunder == map[string]any{
//	//   "ev2": map[string]any{"replicas": "__ev2.replicas__", "name": "__ev2.name__"},
//	//   "registries": "__registries__",
//	// }
func EV2Mapping(input types.Configuration, placeholderGenerator PlaceholderGenerator, prefix []string) (map[string]string, map[string]interface{}) {
	output := map[string]string{}
	replaced := map[string]interface{}{}
	for key, value := range input {
		nestedKey := make([]string, 0, len(prefix)+1)
		nestedKey = append(nestedKey, prefix...)
		nestedKey = append(nestedKey, key)
		if typed, ok := value.(map[string]any); ok {
			flattened, replacement := EV2Mapping(typed, placeholderGenerator, nestedKey)
			for index, what := range flattened {
				output[index] = what
			}
			replaced[key] = replacement
			continue
		}
		flattenedKey, replaceVar := placeholderGenerator(nestedKey, reflect.TypeOf(value))
		output[flattenedKey] = strings.Join(nestedKey, ".")
		replaced[key] = replaceVar
	}
	return output, replaced
}
