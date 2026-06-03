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

// DefaultPlaceholderPattern is the regular expression applied to placeholder
// values when WithAllowPlaceholders is used without an explicit pattern. It
// matches strings of the form "__<anything>__" (dunder-wrapped), which is the
// output format of the sdp-pipelines dunder configuration used during schema
// validation of pipeline.yaml templates.
const DefaultPlaceholderPattern = "^__.+__$"

// ValidateOption configures the behavior of ValidatePipelineSchemaWithOptions
// (and, by extension, NewPipelineFromFile / NewPipelineFromBytes when called
// with options).
type ValidateOption func(*validateOptions)

// validateOptions holds the resolved configuration produced by applying a set
// of ValidateOption values. It is intentionally unexported: callers always
// configure validation through the With* option constructors.
type validateOptions struct {
	// allowPlaceholders, when non-empty, enables placeholder-mode validation.
	// The string is a regular expression that placeholder values must match.
	allowPlaceholders string
}

// newValidateOptions builds a validateOptions by applying the given option
// functions. Options applied later override earlier ones.
func newValidateOptions(opts ...ValidateOption) *validateOptions {
	o := &validateOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	return o
}

// WithAllowPlaceholders enables an opt-in placeholder mode for the pipeline
// schema validator. When this option is supplied, the schema is widened
// in-memory so that every non-string scalar field (boolean / integer / number,
// and pure non-string enums) also accepts a string matching pattern.
//
// This is intended for callers that validate pipeline.yaml content with a
// "dunder configuration" — every leaf rendered as "__<dot.path>__" — where
// real boolean/integer/number fields would otherwise be rejected. The default
// strict validation is unchanged for all other callers, including production
// EV2 manifest generation.
//
// An empty pattern is treated as DefaultPlaceholderPattern ("^__.+__$"), which
// matches sdp-pipelines' dunder output. Callers that template placeholders
// with a different convention may supply their own regular expression (for
// example, "^<<.+>>$").
func WithAllowPlaceholders(pattern string) ValidateOption {
	return func(o *validateOptions) {
		if pattern == "" {
			pattern = DefaultPlaceholderPattern
		}
		o.allowPlaceholders = pattern
	}
}
