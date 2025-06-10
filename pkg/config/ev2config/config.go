package ev2config

import (
	"fmt"

	"sigs.k8s.io/yaml"

	coreconfig "github.com/Azure/ARO-Tools/pkg/config"

	_ "embed"
)

//go:embed config.yaml
var config []byte

func Config() (coreconfig.ConfigurationOverrides, error) {
	ev2Config := coreconfig.NewConfigurationOverrides()
	if err := yaml.Unmarshal(config, &ev2Config); err != nil {
		return nil, fmt.Errorf("failed to parse embedded Ev2 config: %w", err)
	}
	return ev2Config, nil
}
