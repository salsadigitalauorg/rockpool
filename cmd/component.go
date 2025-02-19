package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/salsadigitalauorg/rockpool/pkg/components"
	"github.com/salsadigitalauorg/rockpool/pkg/config"
)

var (
	configFile  string
	rockpoolDir string
	namespace   string
	domain      string
)

var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Manage components",
	Long:  `Install, upgrade, and manage components in your cluster`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Create the default config file.
		err := createDefaultConfigFile()
		if err != nil {
			return fmt.Errorf("failed to create default config: %w", err)
		}

		// Create all the templates to the templates directory.
		err = createTemplates()
		if err != nil {
			return fmt.Errorf("failed to create templates: %w", err)
		}

		return err
	},
}

var componentGenerateConfigCmd = &cobra.Command{
	Use:   "generate-config",
	Short: "Generate default components configuration file",
	Long:  `Generate a components configuration file with all default components from embedded defaults`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Generated default components configuration at %s\n", getDefaultConfigPath())
		return nil
	},
}

var componentInstallCmd = &cobra.Command{
	Use:   "install [component-name]",
	Short: "Install a component",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig(rockpoolDir, configFile)
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
		cfg, err := loadConfig(rockpoolDir, configFile)
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
		cfg, err := loadConfig(rockpoolDir, configFile)
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
		cfg, err := loadConfig(rockpoolDir, configFile)
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
		cfg, err := loadConfig(rockpoolDir, configFile)
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

func getRockpoolDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".rockpool")
}

func getDefaultConfigPath() string {
	return filepath.Join(getRockpoolDir(), "default-components.yaml")
}

func createDefaultConfigFile() error {
	log.Debug().Msgf("creating default config file at %s", getDefaultConfigPath())
	if err := os.MkdirAll(getRockpoolDir(), 0755); err != nil {
		return fmt.Errorf("failed to create rockpool directory: %w", err)
	}

	cfg, err := components.CreateDefaultConfig()
	if err != nil {
		return fmt.Errorf("failed to create default config: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(getDefaultConfigPath(), data, 0644)
}

func createTemplates() error {
	templatesDir := filepath.Join(getRockpoolDir(), "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		return fmt.Errorf("failed to create templates directory: %w", err)
	}

	entries, err := components.DefaultComponents.ReadDir("default/templates")
	if err != nil {
		return fmt.Errorf("failed to read templates: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, err := components.DefaultComponents.ReadFile(filepath.Join("default/templates", entry.Name()))
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", entry.Name(), err)
		}

		dest := filepath.Join(templatesDir, entry.Name())
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return fmt.Errorf("failed to write template %s: %w", entry.Name(), err)
		}
	}

	return nil
}

func loadConfig(rockpoolDir, path string) (*config.Config, error) {
	// If the path is a relative path, concatenate it with the rockpoolDir.
	if !filepath.IsAbs(path) {
		path = filepath.Join(rockpoolDir, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set the rockpoolDir in the config.
	if cfg.Dir == "" {
		cfg.Dir = rockpoolDir
	}

	// Set the domain in the config.
	if cfg.Domain == "" {
		cfg.Domain = domain
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
	componentCmd.PersistentFlags().StringVar(&rockpoolDir, "rockpool-dir",
		getRockpoolDir(), "Path to the rockpool configuration directory")
	componentCmd.PersistentFlags().StringVarP(&configFile, "config", "c",
		getDefaultConfigPath(), "Path to the components configuration file")
	componentCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n",
		"", "Override the namespace for the component")
	componentCmd.PersistentFlags().StringVarP(&domain, "domain", "d",
		"local",
		`The base domain of the platform; ancillary services will be created as
its subdomains using the provided 'name', e.g, rockpool.local,
lagoon.rockpool.local`)
}
