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
	templatePattern = regexp.MustCompile(`{{\s*[^}]+\s*}}`)
	// yamlLinePattern matches YAML field assignments or array items
	// Captures the prefix (including : or -) and the value after it
	// Examples: "key: value" -> captures "key:" and "value"
	//           "- item" -> captures "-" and "item"
	//           "  nested: {{ .template }}" -> captures "  nested:" and "{{ .template }}"
	// Note: Array items are matched first (just the dash) to handle cases like "- item:value"
	yamlLinePattern = regexp.MustCompile(`^(\s*-|\s*[^:]+:)\s*(.*)$`)
)

func WrapFile(inputPath string, outputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", inputPath, err)
	}

	wrapped, err := WrapYAML(data)
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

func WrapYAML(data []byte) ([]byte, error) {
	text := string(data)

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		// skip if already wrapped
		if strings.Contains(line, WrapperMarker) {
			continue
		}

		// ... or if there is no template in the line
		if !templatePattern.MatchString(line) {
			continue
		}

		// Match YAML field assignments or array items
		match := yamlLinePattern.FindStringSubmatch(line)
		if len(match) < 3 {
			continue
		}

		prefix := match[1] // "key:" or "-"
		value := match[2]  // everything after the colon or dash

		// Check if the value contains templates
		if !templatePattern.MatchString(value) {
			continue
		}

		// Parse the existing comment if any
		commentPos := strings.Index(value, "#")
		var valueContent, comment string
		if commentPos != -1 {
			valueContent = strings.TrimSpace(value[:commentPos])
			comment = value[commentPos:]
		} else {
			valueContent = strings.TrimSpace(value)
			comment = ""
		}

		// Only wrap if the value contains templates and is not already quoted
		if templatePattern.MatchString(valueContent) && !isQuoted(valueContent) {
			// Wrap the unquoted value
			wrappedValue := "\"" + valueContent + "\""

			// Add wrapper marker
			if comment != "" {
				lines[i] = prefix + " " + wrappedValue + " " + comment + " " + WrapperMarker
			} else {
				lines[i] = prefix + " " + wrappedValue + " # " + WrapperMarker
			}
		}
	}

	result := []byte(strings.Join(lines, "\n"))

	// Validate that the wrapped output is valid YAML
	var unmarshalTarget any
	err := yaml.Unmarshal(result, &unmarshalTarget)
	if err != nil {
		return nil, fmt.Errorf("wrapped result is not valid YAML: %w", err)
	}

	return result, nil
}

func UnwrapYAML(data []byte) ([]byte, error) {
	text := string(data)

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		// skip lines that don't contain wrapper markers
		if !strings.Contains(line, WrapperMarker) {
			continue
		}

		// Match YAML field assignments or array items
		match := yamlLinePattern.FindStringSubmatch(line)
		if len(match) < 3 {
			continue
		}

		prefix := match[1] // "key:" or "-"
		value := match[2]  // everything after the colon or dash

		// Remove the wrapper marker from the line
		cleanValue := strings.Replace(value, " "+WrapperMarker, "", 1)
		cleanValue = strings.Replace(cleanValue, WrapperMarker, "", 1)
		cleanValue = strings.TrimSpace(cleanValue)

		// Check if there's a comment
		commentPos := strings.Index(cleanValue, "#")
		var valueContent, comment string
		if commentPos != -1 {
			valueContent = strings.TrimSpace(cleanValue[:commentPos])
			comment = cleanValue[commentPos:]
		} else {
			valueContent = strings.TrimSpace(cleanValue)
			comment = ""
		}

		// Remove quotes if the value is quoted (since we only wrap unquoted values)
		if isQuoted(valueContent) {
			// Remove outer quotes
			valueContent = strings.TrimSpace(valueContent)
			if strings.HasPrefix(valueContent, "\"") && strings.HasSuffix(valueContent, "\"") {
				valueContent = valueContent[1 : len(valueContent)-1]
			} else if strings.HasPrefix(valueContent, "'") && strings.HasSuffix(valueContent, "'") {
				valueContent = valueContent[1 : len(valueContent)-1]
			}
		}

		// Reconstruct the line
		if comment != "" && strings.TrimSpace(comment) != "#" {
			lines[i] = prefix + " " + valueContent + " " + comment
		} else {
			lines[i] = prefix + " " + valueContent
		}
	}

	result := []byte(strings.Join(lines, "\n"))
	return result, nil
}

// isQuoted checks if a string is surrounded by quotes
func isQuoted(s string) bool {
	s = strings.TrimSpace(s)
	return (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'"))
}
