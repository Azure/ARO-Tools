package output

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Azure/ARO-Tools/tools/release/client"
	"github.com/Azure/ARO-Tools/tools/release/timeparse"
)

type Format string

const (
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
	FormatHuman Format = "human"
)

func ParseFormat(s string) (Format, error) {
	switch s {
	case "json", "yaml", "human":
		return Format(s), nil
	}
	return "", fmt.Errorf("invalid format: %s", s)
}

func FormatOutput(
	deployments []*client.ReleaseDeployment,
	outputFormat Format,
	loc *time.Location,
	includeComponents bool,
) (string, error) {

	switch outputFormat {
	case FormatJSON:
		jsonBytes, err := json.MarshalIndent(deployments, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to format results: %w", err)
		}
		return string(jsonBytes), nil

	case FormatYAML:
		yamlBytes, err := yaml.Marshal(deployments)
		if err != nil {
			return "", fmt.Errorf("failed to format results: %w", err)
		}
		return string(yamlBytes), nil

	case FormatHuman:
		// Human-readable format
		var b strings.Builder
		fmt.Fprintf(&b, "Found %d deployment(s):\n\n", len(deployments))
		for i, deployment := range deployments {
			rawTimestamp := deployment.Metadata.Timestamp
			timestamp, err := time.Parse(time.RFC3339, rawTimestamp)

			displayTimeStr := fmt.Sprintf("unknown (%s)", rawTimestamp)
			relativeTime := "unknown"
			if err == nil {
				displayTime := timestamp
				if loc != nil {
					displayTime = timestamp.In(loc)
				}
				displayTimeStr = displayTime.Format("2006-01-02 15:04:05 MST")
				relativeTime = timeparse.FormatRelativeTime(time.Since(timestamp))
			}

			fmt.Fprintf(&b, "%d. Deployment to %s was %s ago (%s)\n",
				i+1, deployment.Target.Environment, relativeTime, displayTimeStr)
			fmt.Fprintf(&b, "   Upstream Revision: %s\n", deployment.Metadata.UpstreamRevision)
			fmt.Fprintf(&b, "   Revision: %s\n", deployment.Metadata.Revision)
			fmt.Fprintf(&b, "   Branch: %s\n", deployment.Metadata.Branch)
			if deployment.Metadata.PullRequestID > 0 {
				fmt.Fprintf(&b, "   PR: #%d\n", deployment.Metadata.PullRequestID)
			}
			if len(deployment.Target.RegionConfigs) > 0 {
				fmt.Fprintf(&b, "   Regions: %v\n", deployment.Target.RegionConfigs)
			}
			if includeComponents && len(deployment.Components) > 0 {
				fmt.Fprintf(&b, "   Components: %d\n", len(deployment.Components))
			}
			fmt.Fprintln(&b)
		}
		return b.String(), nil
	default:
		return "", fmt.Errorf("invalid output format: %s", outputFormat)
	}
}
