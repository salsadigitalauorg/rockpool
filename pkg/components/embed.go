package components

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
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

		if err := parseComponentFile(entry.Name(), data, componentsMap); err != nil {
			return nil, err
		}
	}

	// Create the config
	return &config.Config{
		Components: componentsMap,
	}, nil
}

// parseComponentFile decodes a component definition file into componentsMap.
// A single file may contain multiple YAML documents, each defining a separate
// component, so documents are decoded one at a time.
func parseComponentFile(
	fileName string,
	data []byte,
	componentsMap map[string]config.ComponentConfig,
) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	for i := 0; ; i++ {
		var component config.ComponentConfig
		err := decoder.Decode(&component)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to parse component file %s: %w", fileName, err)
		}

		// Use filename without extension as the component name if not
		// specified. Only the first document can take the implicit name,
		// otherwise subsequent documents would silently overwrite it.
		if component.Name == "" {
			if i > 0 {
				return fmt.Errorf("component file %s: document %d has no name",
					fileName, i+1)
			}
			component.Name = strings.TrimSuffix(fileName, filepath.Ext(fileName))
		}

		if existing, ok := componentsMap[component.Name]; ok {
			return fmt.Errorf(
				"duplicate component name %q found in %s (already defined as type %q)",
				component.Name, fileName, existing.Type)
		}

		componentsMap[component.Name] = component
	}
	return nil
}
