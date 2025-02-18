package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/salsadigitalauorg/rockpool/pkg/components"
	"github.com/salsadigitalauorg/rockpool/pkg/config"
)

var (
	configFile string
	namespace  string
)

// getDefaultConfigPath returns the path to the default components config file
func getDefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "rockpool-components.yaml" // fallback to local file if home dir not found
	}
	return filepath.Join(home, ".rockpool", "default-components.yaml")
}

var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Manage components",
	Long:  `Install, upgrade, and manage components in your cluster`,
}

var componentInstallCmd = &cobra.Command{
	Use:   "install [component-name]",
	Short: "Install a component",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig(configFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		componentName := args[0]
		component, ok := cfg.Components[componentName]
		if !ok {
			return fmt.Errorf("component %s not found in config", componentName)
		}

		// Override namespace if provided
		if namespace != "" {
			component.Namespace = namespace
		}

		manager := components.NewComponentManager(cfg, "rockpool")
		return manager.Install(cmd.Context(), component)
	},
}

var componentUpgradeCmd = &cobra.Command{
	Use:   "upgrade [component-name]",
	Short: "Upgrade a component",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig(configFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		componentName := args[0]
		component, ok := cfg.Components[componentName]
		if !ok {
			return fmt.Errorf("component %s not found in config", componentName)
		}

		// Override namespace if provided
		if namespace != "" {
			component.Namespace = namespace
		}

		manager := components.NewComponentManager(cfg, "rockpool")
		return manager.Upgrade(cmd.Context(), component)
	},
}

var componentUninstallCmd = &cobra.Command{
	Use:   "uninstall [component-name]",
	Short: "Uninstall a component",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig(configFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		componentName := args[0]
		component, ok := cfg.Components[componentName]
		if !ok {
			return fmt.Errorf("component %s not found in config", componentName)
		}

		// Override namespace if provided
		if namespace != "" {
			component.Namespace = namespace
		}

		manager := components.NewComponentManager(cfg, "rockpool")
		return manager.Uninstall(cmd.Context(), component)
	},
}

var componentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all components and their status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig(configFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		manager := components.NewComponentManager(cfg, "rockpool")
		fmt.Printf("%-20s %-10s %-10s %-15s %s\n", "NAME", "TYPE", "VERSION", "NAMESPACE", "STATUS")
		for name, component := range cfg.Components {
			installed, err := manager.IsInstalled(cmd.Context(), component)
			status := "Not Installed"
			if err != nil {
				status = fmt.Sprintf("Error: %v", err)
			} else if installed {
				status = "Installed"
			}
			fmt.Printf("%-20s %-10s %-10s %-15s %s\n",
				name,
				component.Type,
				component.Version,
				component.Namespace,
				status)
		}
		return nil
	},
}

var componentStatusCmd = &cobra.Command{
	Use:   "status [component-name]",
	Short: "Show detailed status of a component",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig(configFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		componentName := args[0]
		component, ok := cfg.Components[componentName]
		if !ok {
			return fmt.Errorf("component %s not found in config", componentName)
		}

		manager := components.NewComponentManager(cfg, "rockpool")
		installed, err := manager.IsInstalled(cmd.Context(), component)
		if err != nil {
			return fmt.Errorf("failed to check component status: %w", err)
		}

		fmt.Printf("Component: %s\n", componentName)
		fmt.Printf("Type: %s\n", component.Type)
		fmt.Printf("Version: %s\n", component.Version)
		fmt.Printf("Namespace: %s\n", component.Namespace)
		fmt.Printf("Status: %s\n", map[bool]string{true: "Installed", false: "Not Installed"}[installed])
		if len(component.Dependencies) > 0 {
			fmt.Printf("Dependencies: %v\n", component.Dependencies)
		}
		if component.Type == "kubernetes" {
			fmt.Printf("Manifest Paths:\n")
			for _, path := range component.ManifestPaths {
				fmt.Printf("  - %s\n", path)
			}
		}
		if len(component.Values) > 0 {
			fmt.Printf("Values:\n")
			for k, v := range component.Values {
				fmt.Printf("  %s: %v\n", k, v)
			}
		}
		return nil
	},
}

var componentGenerateConfigCmd = &cobra.Command{
	Use:   "generate-config",
	Short: "Generate default components configuration file",
	Long:  `Generate a components configuration file with all default components from embedded defaults`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get user's home directory
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home directory: %w", err)
		}

		// Create .rockpool directory if it doesn't exist
		rockpoolDir := filepath.Join(home, ".rockpool")
		if err := os.MkdirAll(rockpoolDir, 0755); err != nil {
			return fmt.Errorf("failed to create .rockpool directory: %w", err)
		}

		// Path for the default components config
		configPath := filepath.Join(rockpoolDir, "default-components.yaml")

		// Read all component files from embedded defaults
		componentsMap := make(map[string]config.ComponentConfig)

		entries, err := components.DefaultComponents.ReadDir("default")
		if err != nil {
			return fmt.Errorf("failed to read embedded components: %w", err)
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}

			data, err := components.DefaultComponents.ReadFile(filepath.Join("default", entry.Name()))
			if err != nil {
				return fmt.Errorf("failed to read component file %s: %w", entry.Name(), err)
			}

			var component config.ComponentConfig
			if err := yaml.Unmarshal(data, &component); err != nil {
				return fmt.Errorf("failed to parse component file %s: %w", entry.Name(), err)
			}

			// Use filename without extension as component name if not specified
			if component.Name == "" {
				component.Name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			}

			componentsMap[component.Name] = component
		}

		// Create the config
		cfg := config.Config{
			Components: componentsMap,
		}

		// Marshal the config to YAML
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}

		// Write the config file
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write config file: %w", err)
		}

		fmt.Printf("Generated default components configuration at %s\n", configPath)
		return nil
	},
}

func loadConfig(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set default namespace if not specified
	for name, component := range cfg.Components {
		if component.Namespace == "" {
			component.Namespace = "default"
			cfg.Components[name] = component
		}
	}

	return &cfg, nil
}

func init() {
	rootCmd.AddCommand(componentCmd)
	componentCmd.AddCommand(componentInstallCmd)
	componentCmd.AddCommand(componentUpgradeCmd)
	componentCmd.AddCommand(componentUninstallCmd)
	componentCmd.AddCommand(componentListCmd)
	componentCmd.AddCommand(componentStatusCmd)
	componentCmd.AddCommand(componentGenerateConfigCmd)

	// Add flags
	componentCmd.PersistentFlags().StringVarP(&configFile, "config", "c", getDefaultConfigPath(), "Path to the components configuration file")
	componentCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Override the namespace for the component")
}
