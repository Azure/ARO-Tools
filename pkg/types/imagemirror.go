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

	_ "embed"
)

//go:embed on-demand.sh
var OnDemandSyncScript []byte

// ImageMirrorStep is a handy wrapper over a ShellStep that allows many users of this tooling to mirror images in the
// same way without having to worry about the shell script itself.
type ImageMirrorStep struct {
	StepMeta `json:",inline"`

	TargetACR          Variable `json:"targetACR,omitempty"`
	SourceRegistry     Variable `json:"sourceRegistry,omitempty"`
	Repository         Variable `json:"repository,omitempty"`
	Digest             Variable `json:"digest,omitempty"`
	PullSecretKeyVault Variable `json:"pullSecretKeyVault,omitempty"`
	PullSecretName     Variable `json:"pullSecretName,omitempty"`
	ShellIdentity      Variable `json:"shellIdentity,omitempty"`
}

func (s *ImageMirrorStep) Description() string {
	return fmt.Sprintf("Step %s\n  Kind: %s\n  From %v:%v@%v to %v\n", s.Name, s.Action, s.SourceRegistry, s.Repository, s.Digest, s.TargetACR)
}

// ResolveImageMirrorStep resolves an image mirror step to a shell step. It's up to the user to write the contents of
// the OnDemandSyncScript to disk somewhere and pass the file name in as a parameter here, as we likely don't want to
// inline 100+ lines of shell into a `bash -C "<contents>"` call and hope all the string interpolations work.
func ResolveImageMirrorStep(input ImageMirrorStep, scriptFile string) (*ShellStep, error) {
	return &ShellStep{
		StepMeta: StepMeta{
			Name:   "image-mirror",
			Action: "Shell",
		},
		Command: scriptFile,
		Variables: []Variable{
			namedVariable("TARGET_ACR", input.TargetACR),
			namedVariable("SOURCE_REGISTRY", input.SourceRegistry),
			namedVariable("REPOSITORY", input.Repository),
			namedVariable("DIGEST", input.Digest),
			namedVariable("PULL_SECRET_KV", input.PullSecretKeyVault),
			namedVariable("PULL_SECRET", input.PullSecretName),
		},
		DryRun: DryRun{
			Variables: []Variable{{
				Name:  "DRY_RUN",
				Value: "true",
			}},
		},
		ShellIdentity: input.ShellIdentity,
	}, nil
}

func namedVariable(name string, variable Variable) Variable {
	return Variable{
		Name:      name,
		Value:     variable.Value,
		ConfigRef: variable.ConfigRef,
		Input:     variable.Input,
	}
}

// NewImageMirrorStep creates a new image mirror step.
func NewImageMirrorStep() *ImageMirrorStep {
	return &ImageMirrorStep{
		StepMeta: StepMeta{
			Name:   "image-mirror",
			Action: "ImageMirror",
		},
	}
}

// WithTargetACR fluent method that sets TargetACR.
func (s *ImageMirrorStep) WithTargetACR(targetACR Variable) *ImageMirrorStep {
	s.TargetACR = targetACR
	return s
}

// WithSourceRegistry fluent method that sets SourceRegistry.
func (s *ImageMirrorStep) WithSourceRegistry(sourceRegistry Variable) *ImageMirrorStep {
	s.SourceRegistry = sourceRegistry
	return s
}

// WithRepository fluent method that sets Repository.
func (s *ImageMirrorStep) WithRepository(repository Variable) *ImageMirrorStep {
	s.Repository = repository
	return s
}

// WithDigest fluent method that sets Digest.
func (s *ImageMirrorStep) WithDigest(digest Variable) *ImageMirrorStep {
	s.Digest = digest
	return s
}

// WithPullSecretKeyVault fluent method that sets PullSecretKeyVault.
func (s *ImageMirrorStep) WithPullSecretKeyVault(pullSecretKeyVault Variable) *ImageMirrorStep {
	s.PullSecretKeyVault = pullSecretKeyVault
	return s
}

// WithPullSecretName fluent method that sets PullSecretName.
func (s *ImageMirrorStep) WithPullSecretName(pullSecretName Variable) *ImageMirrorStep {
	s.PullSecretName = pullSecretName
	return s
}

// WithShellIdentity fluent method that sets ShellIdentity.
func (s *ImageMirrorStep) WithShellIdentity(shellIdentity Variable) *ImageMirrorStep {
	s.ShellIdentity = shellIdentity
	return s
}
