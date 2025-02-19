package components

import (
	"embed"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/salsadigitalauorg/rockpool/pkg/config"
)

//go:embed default/*.yaml default/templates/*
var DefaultComponents embed.FS

func CreateDefaultConfig() (*config.Config, error) {
	// Read all component files from embedded defaults
	componentsMap := make(map[string]config.ComponentConfig)

	entries, err := DefaultComponents.ReadDir("default")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded components: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		data, err := DefaultComponents.ReadFile(filepath.Join("default", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read component file %s: %w", entry.Name(), err)
		}

		var component config.ComponentConfig
		if err := yaml.Unmarshal(data, &component); err != nil {
			return nil, fmt.Errorf("failed to parse component file %s: %w", entry.Name(), err)
		}

		// Use filename without extension as component name if not specified
		if component.Name == "" {
			component.Name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}

		componentsMap[component.Name] = component
	}

	// Create the config
	return &config.Config{
		Components: componentsMap,
	}, nil
}
