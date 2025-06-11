package ev2config

import coreconfig "github.com/Azure/ARO-Tools/pkg/config"

type config struct {
	Clouds map[string]SanitizedCloudConfig `json:"clouds"`
}

type SanitizedCloudConfig struct {
	Defaults coreconfig.Configuration            `json:"defaults"`
	Regions  map[string]coreconfig.Configuration `json:"regions"`
}
