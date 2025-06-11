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

import "fmt"

type LogsStep struct {
	StepMeta        `json:",inline"`
	SubscriptionId  Variable   `json:"subscriptionId,omitempty"`
	Namespace       Variable   `json:"namespace,omitempty"`
	CertSAN         Variable   `json:"certsan,omitempty"`
	CertDescription Variable   `json:"certdescription,omitempty"`
	ConfigVersion   Variable   `json:"configVersion,omitempty"`
	Events          LogsEvents `json:"events,omitempty"`
}

type LogsEvents struct {
	AKSKubeSystem string `json:"akskubesystem,omitempty"`
}

func NewRPLogsAccountStep(name string) *LogsStep {
	return &LogsStep{
		StepMeta: StepMeta{
			Name:   name,
			Action: "RPLogsAccount",
		},
	}
}

func NewClusterLogsAccountStep(name string) *LogsStep {
	return &LogsStep{
		StepMeta: StepMeta{
			Name:   name,
			Action: "ClusterLogsAccount",
		},
	}
}

// WithDependsOn fluent method that sets DependsOn
func (s *LogsStep) WithDependsOn(dependsOn ...string) *LogsStep {
	s.DependsOn = dependsOn
	return s
}

// WithSubscriptionId fluent method that sets subscriptionId
func (s *LogsStep) WithSubscriptionId(subscriptionId Variable) *LogsStep {
	s.SubscriptionId = subscriptionId
	return s
}

// WithNamespace fluent method that sets namespace
func (s *LogsStep) WithNamespace(namespace Variable) *LogsStep {
	s.Namespace = namespace
	return s
}

// WithCertSAN fluent method that sets certSAN
func (s *LogsStep) WithCertSAN(certSAN Variable) *LogsStep {
	s.CertSAN = certSAN
	return s
}

// WithCertDescription fluent method that sets certDescription
func (s *LogsStep) WithCertDescription(certDescription Variable) *LogsStep {
	s.CertDescription = certDescription
	return s
}

// WithConfigVersion fluent method that sets configVersion
func (s *LogsStep) WithConfigVersion(configVersion Variable) *LogsStep {
	s.ConfigVersion = configVersion
	return s
}

// WithEvents fluent method that sets events
func (s *LogsStep) WithEvents(aksKubeSystem string) *LogsStep {
	s.Events = LogsEvents{
		AKSKubeSystem: aksKubeSystem,
	}
	return s
}

func (s *LogsStep) Description() string {
	return fmt.Sprintf("Step %s\n Kind: %s\n", s.Name, s.Action)
}
