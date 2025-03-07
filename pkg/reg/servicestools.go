package reg

import (
	"context"
	"fmt"
	"os"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"gopkg.in/yaml.v2"

	"santoshkal/mcp-godocker/pkg/mcp"
)

// Config represents the overall YAML configuration.
type Config struct {
	Services []ServiceConfig `yaml:"services"`
}

// ServiceConfig defines a service entry.
type ServiceConfig struct {
	Name    string       `yaml:"name"`
	Enabled bool         `yaml:"enabled"` // if false, skip this service
	Tools   []ToolConfig `yaml:"tools"`
}

// ToolConfig defines an individual tool.
type ToolConfig struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Enabled     bool                   `yaml:"enabled"` // if false, skip this tool
	Schema      map[string]interface{} `yaml:"schema"`
	Script      string                 `yaml:"script"` // Inline Go code for the handler.
}

// loadConfig reads and unmarshals the YAML file.
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}
	return &cfg, nil
}

// RegisterToolsFromConfig loads the configuration, evaluates each tool's script using yaegi,
// and registers only the enabled tools using the provided Registry.
func RegisterToolsFromConfig(r mcp.Registry, configPath string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	for _, svc := range cfg.Services {
		if !svc.Enabled {
			continue
		}
		for _, tool := range svc.Tools {
			if !tool.Enabled {
				continue
			}

			// Create a new yaegi interpreter instance.
			i := interp.New(interp.Options{})
			i.Use(stdlib.Symbols)

			// Evaluate the provided script. The script must define a function "Handler".
			_, err := i.Eval(tool.Script)
			if err != nil {
				return fmt.Errorf("failed to evaluate script for tool %s: %v", tool.Name, err)
			}

			// Retrieve the Handler symbol.
			v, err := i.Eval("Handler")
			if err != nil {
				return fmt.Errorf("failed to retrieve Handler symbol for tool %s: %v", tool.Name, err)
			}

			// Assert that the symbol has the correct signature.
			handler, ok := v.Interface().(func(context.Context, mcp.Registry, map[string]interface{}) error)
			if !ok {
				return fmt.Errorf("handler for tool %s does not have the correct signature", tool.Name)
			}

			// Register the tool.
			r.RegisterTool(tool.Name, tool.Description, tool.Schema, handler)
		}
	}
	return nil
}
