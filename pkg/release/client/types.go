package client

import "fmt"

type Environment string

const (
	ProdEnv Environment = "prod"
	StgEnv  Environment = "stg"
	IntEnv  Environment = "int"
)

func ParseEnvironment(s string) (Environment, error) {
	switch s {
	case "prod", "stg", "int":
		return Environment(s), nil
	}
	return "", fmt.Errorf("invalid environment: %s", s)
}

// ReleaseMetadata describes how and when a release was created
type ReleaseMetadata struct {
	UpstreamRevision string `json:"upstreamRevision" yaml:"upstreamRevision"`
	Revision         string `json:"revision" yaml:"revision"`
	Branch           string `json:"branch" yaml:"branch"`
	Timestamp        string `json:"timestamp" yaml:"timestamp"`
	PullRequestID    int    `json:"pullRequestId" yaml:"pullRequestId"`
	ServiceGroup     string `json:"serviceGroup" yaml:"serviceGroup"`
	ServiceGroupBase string `json:"serviceGroupBase" yaml:"serviceGroupBase"`
}

// DeploymentTarget describes where a release is being deployed
type DeploymentTarget struct {
	Cloud         string      `json:"cloud" yaml:"cloud"`
	Environment   Environment `json:"environment" yaml:"environment"`
	RegionConfigs []string    `json:"regionConfigs" yaml:"regionConfigs"`
}

// Components is a map of component names to their image digests
type Components map[string]string

// ReleaseDeployment represents deploying a Release to a specific Target
type ReleaseDeployment struct {
	Metadata   ReleaseMetadata   `json:"metadata" yaml:"metadata"`
	Target     DeploymentTarget  `json:"target" yaml:"target"`
	Components map[string]string `json:"components,omitempty" yaml:"components,omitempty"`
}

func (rd *ReleaseDeployment) UnmarshalYAML(unmarshal func(any) error) error {
	// current file structure for release.yaml
	var fileData struct {
		Branch           string   `yaml:"branch"`
		Timestamp        string   `yaml:"timestamp"`
		PullRequestID    int      `yaml:"pullRequestId"`
		Revision         string   `yaml:"revision"`
		UpstreamRevision string   `yaml:"upstreamRevision"`
		Cloud            string   `yaml:"cloud"`
		Environment      string   `yaml:"environment"`
		RegionConfigs    []string `yaml:"regionConfigs"`
		ServiceGroupBase string   `yaml:"serviceGroupBase"`
		ServiceGroup     string   `yaml:"serviceGroup"`
	}

	if err := unmarshal(&fileData); err != nil {
		return err
	}

	// Map to ReleaseDeployment structure
	rd.Metadata = ReleaseMetadata{
		UpstreamRevision: fileData.UpstreamRevision,
		Revision:         fileData.Revision,
		Branch:           fileData.Branch,
		Timestamp:        fileData.Timestamp,
		PullRequestID:    fileData.PullRequestID,
		ServiceGroup:     fileData.ServiceGroup,
		ServiceGroupBase: fileData.ServiceGroupBase,
	}

	env, err := ParseEnvironment(fileData.Environment)
	if err != nil {
		return err
	}
	rd.Target = DeploymentTarget{
		Cloud:         fileData.Cloud,
		Environment:   env,
		RegionConfigs: fileData.RegionConfigs,
	}

	rd.Components = make(Components)

	return nil
}

type ReleaseDeploymentList struct {
	Items []ReleaseDeployment `json:"items" yaml:"items"`
}
