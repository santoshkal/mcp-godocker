package reg

import (
	"context"
	"fmt"
	"os"
	"plugin"

	"gopkg.in/yaml.v2"

	"github.com/santoshkal/mcp-godocker/pkg/server"
)

// Config structures for YAML.
type Config struct {
	Services []ServiceConfig `yaml:"services"`
}

type ServiceConfig struct {
	Name  string       `yaml:"name"`
	Tools []ToolConfig `yaml:"tools"`
}

type ToolConfig struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Schema      map[string]interface{} `yaml:"schema"`
	Plugin      PluginConfig           `yaml:"plugin"`
}

type PluginConfig struct {
	Path   string `yaml:"path"`
	Symbol string `yaml:"symbol"`
}

// loadConfig reads and unmarshals the YAML file.
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading YAML config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling YAML: %w", err)
	}
	return &cfg, nil
}

// RegisterToolsFromConfig loads the configuration from YAML,
// loads the plugin for each tool, and registers the tool with the server.
func RegisterToolsFromConfig(s *server.Server, configPath string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	for _, svc := range cfg.Services {
		for _, tool := range svc.Tools {
			p, err := plugin.Open(tool.Plugin.Path)
			if err != nil {
				return fmt.Errorf("failed to open plugin %s: %v", tool.Plugin.Path, err)
			}

			symbol, err := p.Lookup(tool.Plugin.Symbol)
			if err != nil {
				return fmt.Errorf("failed to lookup symbol %s in plugin %s: %v", tool.Plugin.Symbol, tool.Plugin.Path, err)
			}

			// Assert that the symbol implements the expected type.
			handler, ok := symbol.(func(context.Context, *server.Server, map[string]interface{}) error)
			if !ok {
				return fmt.Errorf("plugin symbol %s in %s does not match expected handler signature", tool.Plugin.Symbol, tool.Plugin.Path)
			}

			// Register the tool dynamically.
			s.RegisterTool(tool.Name, tool.Description, tool.Schema, handler)
		}
	}
	return nil
}
