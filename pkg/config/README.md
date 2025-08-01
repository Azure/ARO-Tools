# Configuration System

The ARO-Tools configuration system provides a flexible, template-based approach to managing service configurations across multiple clouds, environments, and regions. It uses a two-stage processing design to enable both configuration discovery and context-specific resolution.

## Overview

Configuration files in this system are **Go templates**, not plain YAML. This allows for dynamic configuration values based on deployment context (cloud, environment, region) while maintaining a single source of truth.

### Key Concepts

- **Context**: A specific combination of cloud, environment, and region (e.g., "public/int/uksouth")
- **Template**: Configuration files with Go template syntax (e.g., `{{.ctx.cloud}}`, `{{.ev2.minReplicas}}`)
- **Provider**: Discovers available contexts from configuration templates
- **Resolver**: Resolves actual configuration values for a specific context

## Two-Stage Design

The system uses a two-stage approach to handle the chicken-and-egg problem of needing to discover available contexts before users specify their target context.

### Stage 1: Discovery (NewConfigProvider)

```go
provider, err := config.NewConfigProvider("config.yaml")
```

**Purpose**: Parse template structure with dummy values to discover available contexts

**Process**:
1. Reads the configuration template file
2. Processes template with **dummy values**:
   - `CloudReplacement: "public"`
   - `EnvironmentReplacement: "int"`
   - `RegionReplacement: "uksouth"`
   - `StampReplacement: "1"`
3. Stores processed structure for context discovery
4. **Does NOT** provide real configuration values

**Why dummy values?**: Templates cannot be parsed as YAML until variables are replaced. Dummy values allow structural parsing without requiring user's target context.

### Stage 2: Resolution (GetResolver)

```go
resolver, err := provider.GetResolver(&config.ConfigReplacements{
    CloudReplacement:       "government",
    EnvironmentReplacement: "prod", 
    RegionReplacement:      "usgovvirginia",
    // ... other values
})
```

**Purpose**: Process template with real values for specific deployment context

**Process**:
1. Validates required context values (cloud, environment)
2. Processes template with **real user-provided values**
3. Creates resolver with context-specific configuration
4. Provides actual configuration values for deployment

## Workflow Examples

### Basic Usage

```go
package main

import (
    "fmt"
    "github.com/Azure/ARO-Tools/pkg/config"
    "github.com/Azure/ARO-Tools/pkg/config/ev2config"
)

func main() {
    // Stage 1: Create provider and discover available contexts
    provider, err := config.NewConfigProvider("config.yaml")
    if err != nil {
        panic(err)
    }
    
    // Discover what contexts are available
    contexts := provider.AllContexts()
    fmt.Println("Available contexts:", contexts)
    // Output: map[public:map[int:[uksouth eastus] prod:[uksouth eastus westeurope]] government:map[prod:[usgovvirginia]]]
    
    // Stage 2: Choose specific context and get resolver
    ev2Config, err := ev2config.ResolveConfig("public", "uksouth")
    if err != nil {
        panic(err)
    }
    
    resolver, err := provider.GetResolver(&config.ConfigReplacements{
        CloudReplacement:       "public",
        EnvironmentReplacement: "int",
        RegionReplacement:      "uksouth", 
        RegionShortReplacement: "uks",
        StampReplacement:       "1",
        Ev2Config:              ev2Config,
    })
    if err != nil {
        panic(err)
    }
    
    // Get configuration for the specific context
    cfg, err := resolver.GetRegionConfiguration("uksouth")
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Configuration for public/int/uksouth: %+v\n", cfg)
}
```

### Multi-Region Deployment

```go
// Get all available regions for a cloud/environment
regions, err := resolver.GetRegions()
if err != nil {
    panic(err)
}

// Deploy to all regions
for _, region := range regions {
    cfg, err := resolver.GetRegionConfiguration(region)
    if err != nil {
        panic(err)
    }
    
    // Deploy with region-specific configuration
    deploy(region, cfg)
}
```

### Configuration Validation

```go
// Validate configuration against JSON schema
cfg, err := resolver.GetRegionConfiguration("uksouth")
if err != nil {
    panic(err)
}

err = resolver.ValidateSchema(cfg)
if err != nil {
    panic(fmt.Errorf("configuration validation failed: %w", err))
}
```

## Configuration File Structure

Configuration files use a hierarchical override structure:

```yaml
# config.yaml (template)
$schema: "./config.schema.json"

defaults:
  database_url: "default-{{.ctx.cloud}}.database.com"
  replica_count: {{.ev2.minReplicas}}

clouds:
  public:
    defaults:
      database_url: "public-{{.ctx.environment}}.database.com"
    environments:
      int:
        defaults:
          replica_count: 2
        regions:
          uksouth:
            database_url: "public-int-uksouth.database.com"
          eastus:
            replica_count: 3
      prod:
        defaults:
          replica_count: {{.ev2.maxReplicas}}
        regions:
          uksouth: {}
          eastus: {}
          westeurope: {}
  government:
    defaults:
      database_url: "gov-{{.ctx.environment}}.database.com"
    environments:
      prod:
        defaults:
          replica_count: {{.ev2.govReplicas}}
        regions:
          usgovvirginia:
            database_url: "gov-prod-usgovvirginia.database.com"
```

## Template Variables

Templates have access to:

- **Context variables** (`{{.ctx.*}}`):
  - `{{.ctx.cloud}}`: Cloud name (e.g., "public", "government")
  - `{{.ctx.environment}}`: Environment (e.g., "int", "prod")
  - `{{.ctx.region}}`: Region (e.g., "uksouth", "eastus")
  - `{{.ctx.regionShort}}`: Short region code (e.g., "uks", "eus")
  - `{{.ctx.stamp}}`: Stamp identifier

- **EV2 variables** (`{{.ev2.*}}`):
  - Values from EV2 central configuration
  - Context-specific based on cloud and region

## Configuration Resolution Order

Values are merged in this priority order (later overrides earlier):

1. **Global defaults** (`defaults`)
2. **Cloud defaults** (`clouds.{cloud}.defaults`)
3. **Environment defaults** (`clouds.{cloud}.environments.{env}.defaults`)
4. **Region overrides** (`clouds.{cloud}.environments.{env}.regions.{region}`)

## Error Handling

The system provides detailed error messages for common issues:

```go
// Missing required context
resolver, err := provider.GetResolver(&config.ConfigReplacements{
    // Missing CloudReplacement
    EnvironmentReplacement: "int",
})
// Error: "cloud" override is required

// Invalid context
cfg, err := resolver.GetRegionConfiguration("nonexistent")
// Error: the cloud nonexistent is not found in the config
```

## Best Practices

1. **Validate schemas**: Use `ValidateSchema()` to catch configuration errors early  
1. **Handle missing regions gracefully**: Region configurations are optional and fall back to environment defaults
1. **Use consistent naming**: Follow cloud/environment/region naming conventions
1. **Test templates**: Ensure templates work with all expected contexts

## Related Packages

- [`pkg/config/ev2config`](ev2config/): EV2 central configuration management
- [`pkg/config/types`](types/): Configuration type definitions
- [`pkg/types`](../types/): Pipeline and other type definitions
