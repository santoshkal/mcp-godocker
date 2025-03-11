package reg

import (
	"bytes"
	"context"
	"fmt"
	"go/build"
	"os"

	_ "github.com/docker/docker/api/types/network"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"gopkg.in/yaml.v2"

	"santoshkal.com/mcp-godocker/pkg/mcp"
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
			goPath := build.Default.GOPATH + "/pkg/mod"
			// goPath := os.Getenv("GOPATH")
			// if goPath == "" {
			// 	goPath = "/go"
			// }
			fmt.Printf("GoPath: %v\n", goPath)
			// Create a new yaegi interpreter instance.
			var stdout, stderr bytes.Buffer
			i := interp.New(interp.Options{GoPath: goPath, Stdout: &stdout, Stderr: &stderr})
			// i := interp.New(interp.Options{})
			if err := i.Use(stdlib.Symbols); err != nil {
				fmt.Printf("error loading package symbols: %v", err)
			}
			// if err := i.Use(interp.Symbols); err != nil {
			// 	fmt.Printf("error loading exported symbols: %v", err)
			// }
			// _, err := i.Eval(`import (
			// 	"github.com/docker/docker/api/types/network"
			// 	"santoshkal.com/mcp-godocker/pkg/mcp"
			// 	)`)
			// if err != nil {
			// 	fmt.Printf("error loading docker symbols: %v", err)
			// }
			// if _, err := i.Eval("import \"santoshkal/mcp-godocker/pkg/mcp\""); err != nil {
			// 	fmt.Printf("error loading package symbols: %v", err)
			// }
			// if err := i.Use(interp.Exports{
			// 	"santoshkal/mcp-godocker/pkg/mcp": {
			// 		"Registry":    reflect.ValueOf((mcp.Registry)(nil)),
			// 		"ToolHandler": reflect.ValueOf((mcp.ToolHandler)(nil)),
			// 	},
			// 	// If needed, also export symbols for third-party packages:
			// }); err != nil {
			// 	fmt.Printf("error loading package symbols: %v", err)
			// }
			// if err := i.Use(interp.Exports{
			// 	"github.com/docker/docker/api/types/network": {
			// 		// Export the symbols you need from the docker package, e.g.:
			// 		"CreateOptions": reflect.ValueOf(network.CreateOptions{}),
			// 	},
			// }); err != nil {
			// 	fmt.Printf("error loading package symbols: %v", err)
			// }

			// ImportUsed compiles and imports the used packages.
			// i.ImportUsed()
			// Evaluate the provided script. The script must define a function "Handler".
			if _, err := i.Eval(tool.Script); err != nil {
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
