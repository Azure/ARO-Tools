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

package yamlwrap

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

const (
	// WrapperMarker is the comment marker that identifies wrapped template values
	WrapperMarker = "__WRAPPED_TEMPLATE__"
)

var (
	// templatePattern matches Go template expressions like {{ .foo.bar }}
	// Examples: {{ .value }}, {{.Values.name}}, {{ range .items }}
	templatePattern = regexp.MustCompile(`{{[^}]+}}`)

	// yamlLinePattern matches YAML key-value pairs and array items
	// Captures: prefix (key: or - or - key:) and the rest of the line
	// Examples: "key: value", "- item", "- key: value"
	yamlLinePattern = regexp.MustCompile(`^(\s*(?:[^:\s]+:\s*|-\s+(?:[^:\s]+:\s*)?))(.*)$`)
)

func WrapFile(inputPath string, outputPath string, validateResult bool) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", inputPath, err)
	}

	wrapped, err := WrapYAML(data, validateResult)
	if err != nil {
		return fmt.Errorf("failed to wrap YAML: %w", err)
	}

	err = os.WriteFile(outputPath, wrapped, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", outputPath, err)
	}

	return nil
}

func UnwrapFile(inputPath string, outputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", inputPath, err)
	}

	unwrapped, err := UnwrapYAML(data)
	if err != nil {
		return fmt.Errorf("failed to unwrap YAML: %w", err)
	}

	err = os.WriteFile(outputPath, unwrapped, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", outputPath, err)
	}

	return nil
}

func WrapYAML(data []byte, validateResult bool) ([]byte, error) {
	lines := strings.Split(string(data), "\n")

	for i, line := range lines {
		// Skip if already wrapped
		if strings.Contains(line, WrapperMarker) {
			continue
		}

		// Match YAML structure
		match := yamlLinePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		prefix := match[1]
		value := match[2]

		// Split value and comment
		commentIdx := strings.Index(value, "#")
		var valueContent, comment string
		if commentIdx >= 0 {
			valueContent = strings.TrimSpace(value[:commentIdx])
			comment = value[commentIdx:]
		} else {
			valueContent = strings.TrimSpace(value)
		}

		// Only process if value has template and is not quoted
		if valueContent != "" && templatePattern.MatchString(valueContent) && !isQuoted(valueContent) {
			wrappedValue := fmt.Sprintf(`"%s"`, valueContent)

			if comment != "" {
				lines[i] = fmt.Sprintf("%s%s %s %s", prefix, wrappedValue, comment, WrapperMarker)
			} else {
				lines[i] = fmt.Sprintf("%s%s # %s", prefix, wrappedValue, WrapperMarker)
			}
		}
	}

	result := []byte(strings.Join(lines, "\n"))

	// Validate if requested
	if validateResult {
		var unmarshalTarget any
		if err := yaml.Unmarshal(result, &unmarshalTarget); err != nil {
			return nil, fmt.Errorf("wrapped result is not valid YAML: %w", err)
		}
	}

	return result, nil
}

func UnwrapYAML(data []byte) ([]byte, error) {
	lines := strings.Split(string(data), "\n")

	for i, line := range lines {
		// Skip lines without wrapper markers
		if !strings.Contains(line, WrapperMarker) {
			continue
		}

		// Match YAML structure
		match := yamlLinePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		prefix := match[1]
		value := match[2]

		// Remove wrapper marker
		value = strings.Replace(value, " "+WrapperMarker, "", 1)
		value = strings.Replace(value, WrapperMarker, "", 1)

		// Split value and comment
		commentIdx := strings.Index(value, "#")
		var valueContent, comment string
		if commentIdx >= 0 {
			valueContent = strings.TrimSpace(value[:commentIdx])
			comment = value[commentIdx:]
		} else {
			valueContent = strings.TrimSpace(value)
		}

		// Remove quotes if present
		if isQuoted(valueContent) {
			valueContent = valueContent[1 : len(valueContent)-1]
		}

		// Reconstruct line
		if comment != "" && strings.TrimSpace(comment) != "#" {
			lines[i] = fmt.Sprintf("%s%s %s", prefix, valueContent, comment)
		} else {
			lines[i] = prefix + valueContent
		}
	}

	return []byte(strings.Join(lines, "\n")), nil
}

// isQuoted checks if a string is surrounded by quotes
func isQuoted(s string) bool {
	s = strings.TrimSpace(s)
	return (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'"))
}
