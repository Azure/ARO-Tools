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
	"reflect"
	"strconv"
	"strings"
)

// widenScalarsForPlaceholders rewrites a decoded JSON schema map in-place so
// that every non-string scalar declaration (type boolean / integer / number,
// pure non-string enums, and non-string consts) also accepts a string matching
// pattern.
//
// The rewrite preserves the original constraints by wrapping each scalar node
// in a oneOf[<original-node-clone>, {type:"string", pattern:<pattern>}]. The
// walker recurses through all JSON Schema combinator and subtree keywords,
// and resolves local "$ref" references (e.g. "#/definitions/foo") against the
// root schema so widening still applies to scalars defined under
// $defs/definitions. A set of already-processed map identities prevents both
// $ref cycles and double-widening (where the same scalar is reachable through
// both a $ref and a direct definitions descent).
//
// schemaMap is the root schema and is mutated in place. Pattern is the regex
// the placeholder string must match (typically DefaultPlaceholderPattern).
func widenScalarsForPlaceholders(schemaMap map[string]interface{}, pattern string) {
	if schemaMap == nil || pattern == "" {
		return
	}
	processed := map[uintptr]bool{}
	widenNode(schemaMap, schemaMap, pattern, processed)
}

// scalarTypesToWiden is the set of non-string scalar type names whose
// declarations get widened by widenNode. "null" is intentionally excluded
// (placeholders are never null) and "string" is excluded (already a string).
var scalarTypesToWiden = map[string]bool{
	"boolean": true,
	"integer": true,
	"number":  true,
}

// widenNode mutates a single schema node in place. If the node declares a
// widenable scalar type (a pure non-string enum, or a non-string const), it is
// replaced with a oneOf wrapper. Otherwise the walker recurses into combinator and subtree
// keywords. Local $ref targets are resolved against root. Each map node is
// processed at most once; revisits via $ref or duplicated tree paths are
// no-ops, which prevents the produced oneOf wrapper from being wrapped
// again by a later traversal.
func widenNode(node interface{}, root map[string]interface{}, pattern string, processed map[uintptr]bool) {
	switch n := node.(type) {
	case map[string]interface{}:
		ptr := reflect.ValueOf(n).Pointer()
		if processed[ptr] {
			return
		}
		processed[ptr] = true

		if ref, ok := n["$ref"].(string); ok {
			if target := resolveLocalRef(root, ref); target != nil {
				widenNode(target, root, pattern, processed)
			}
		}

		if shouldWidenNode(n) {
			wrapWithPlaceholderOneOf(n, pattern, processed)
			return
		}

		recurseSubtrees(n, root, pattern, processed)
	case []interface{}:
		for _, item := range n {
			widenNode(item, root, pattern, processed)
		}
	}
}

// shouldWidenNode reports whether obj declares a non-string scalar that should
// be widened into a oneOf wrapper. Scalar declarations include direct type
// declarations (boolean, integer, number — and type-union variants without
// "string"), pure non-string enums, and non-string consts.
func shouldWidenNode(obj map[string]interface{}) bool {
	switch t := obj["type"].(type) {
	case string:
		if scalarTypesToWiden[t] {
			return true
		}
	case []interface{}:
		hasString := false
		hasWidenable := false
		for _, v := range t {
			if s, ok := v.(string); ok {
				if s == "string" {
					hasString = true
				}
				if scalarTypesToWiden[s] {
					hasWidenable = true
				}
			}
		}
		if hasWidenable && !hasString {
			return true
		}
	}
	if c, hasConst := obj["const"]; hasConst {
		if _, isString := c.(string); !isString {
			return true
		}
		return false
	}
	if _, hasType := obj["type"]; hasType {
		return false
	}
	if enum, ok := obj["enum"].([]interface{}); ok && len(enum) > 0 {
		for _, v := range enum {
			if _, isString := v.(string); isString {
				return false
			}
		}
		return true
	}
	return false
}

// wrapWithPlaceholderOneOf converts obj in-place into a oneOf wrapper. The
// original constraints are copied unchanged into the first oneOf alternative
// and a {type:"string",pattern} branch is added as the second alternative.
// The freshly-created original-alternative map is registered in processed so
// that any later traversal reaching it (for example via repeated tree
// descent) will skip it, preventing recursive widening of the same scalar.
func wrapWithPlaceholderOneOf(obj map[string]interface{}, pattern string, processed map[uintptr]bool) {
	original := make(map[string]interface{}, len(obj))
	for k, v := range obj {
		original[k] = v
		delete(obj, k)
	}
	processed[reflect.ValueOf(original).Pointer()] = true

	placeholder := map[string]interface{}{
		"type":    "string",
		"pattern": pattern,
	}
	processed[reflect.ValueOf(placeholder).Pointer()] = true

	obj["oneOf"] = []interface{}{original, placeholder}
}

// schemaSubtreeKeys lists JSON Schema keywords whose values contain further
// subschemas (objects or arrays of objects). The widener recurses into each
// to find scalar declarations to widen.
var schemaSubtreeKeys = []string{
	"properties",
	"patternProperties",
	"$defs",
	"definitions",
	"items",
	"prefixItems",
	"additionalProperties",
	"unevaluatedProperties",
	"unevaluatedItems",
	"contains",
	"not",
	"if",
	"then",
	"else",
	"propertyNames",
	"oneOf",
	"anyOf",
	"allOf",
	"dependentSchemas",
	"dependencies",
}

// recurseSubtrees walks the standard JSON Schema subtree containers in obj
// and invokes widenNode on each inner schema. The set of keys whose values
// are maps-of-schemas (rather than a single schema) is handled specially.
func recurseSubtrees(obj map[string]interface{}, root map[string]interface{}, pattern string, processed map[uintptr]bool) {
	for _, key := range schemaSubtreeKeys {
		child, ok := obj[key]
		if !ok {
			continue
		}
		switch v := child.(type) {
		case map[string]interface{}:
			switch key {
			case "properties", "patternProperties", "$defs", "definitions", "dependentSchemas":
				for _, sub := range v {
					widenNode(sub, root, pattern, processed)
				}
			default:
				widenNode(v, root, pattern, processed)
			}
		case []interface{}:
			for _, item := range v {
				widenNode(item, root, pattern, processed)
			}
		case bool:
			// "additionalProperties: true|false" etc — nothing to widen.
		}
	}
}

// resolveLocalRef resolves a JSON pointer of the form "#/definitions/foo"
// against the root schema. Returns nil for non-local refs (external URLs) or
// any pointer that does not resolve. Numeric path components select array
// indices when applicable.
func resolveLocalRef(root map[string]interface{}, ref string) interface{} {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
	var current interface{} = root
	for _, p := range parts {
		p = unescapeJSONPointer(p)
		switch node := current.(type) {
		case map[string]interface{}:
			next, ok := node[p]
			if !ok {
				return nil
			}
			current = next
		case []interface{}:
			n, err := strconv.Atoi(p)
			if err != nil {
				return nil
			}
			if n < 0 || n >= len(node) {
				return nil
			}
			current = node[n]
		default:
			return nil
		}
	}
	return current
}

// unescapeJSONPointer decodes the two RFC 6901 escape sequences (~1 -> /,
// ~0 -> ~) used in JSON pointer fragments.
func unescapeJSONPointer(p string) string {
	p = strings.ReplaceAll(p, "~1", "/")
	p = strings.ReplaceAll(p, "~0", "~")
	return p
}
