package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/salsadigitalauorg/rockpool/pkg/components"
	"github.com/salsadigitalauorg/rockpool/pkg/config"
	"github.com/salsadigitalauorg/rockpool/pkg/rockpool"
)

var (
	configFile    string
	rockpoolDir   string
	namespace     string
	domain        string
	dnsIp         string
	lagoonVersion string
	forceGenerate bool
)

var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Manage components",
	Long:  `Install, upgrade, and manage components in your cluster`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Deliberately does not call rootCmd.PersistentPreRun: that runs the
		// legacy k3d initialisation, which hard-fails when k3d or the lagoon
		// CLI are absent. Components only need kubectl and helm, and operate
		// on the current kubectl context.
		setupLogging()
		rockpool.EnsureComponentBinariesExist()

		// Scaffold the default config file if it does not exist yet. Existing
		// files are never overwritten here; use 'generate-config --force' to
		// deliberately regenerate them.
		err := createDefaultConfigFile(false)
		if err != nil {
			return fmt.Errorf("failed to create default config: %w", err)
		}

		// Scaffold any missing templates in the templates directory.
		err = createTemplates(false)
		if err != nil {
			return fmt.Errorf("failed to create templates: %w", err)
		}

		return nil
	},
}

var componentGenerateConfigCmd = &cobra.Command{
	Use:   "generate-config",
	Short: "Generate default components configuration file",
	Long: `Generate a components configuration file with all default components from
embedded defaults, along with their templates.

By default, existing files are preserved. Use --force to overwrite them with the
embedded defaults, discarding any local modifications.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := createDefaultConfigFile(forceGenerate); err != nil {
			return fmt.Errorf("failed to create default config: %w", err)
		}

		if err := createTemplates(forceGenerate); err != nil {
			return fmt.Errorf("failed to create templates: %w", err)
		}

		if forceGenerate {
			fmt.Printf("Regenerated default components configuration at %s\n",
				getDefaultConfigPath())
		} else {
			fmt.Printf("Ensured default components configuration exists at %s\n"+
				"Existing files were preserved; use --force to overwrite them.\n",
				getDefaultConfigPath())
		}
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

		manager := components.NewComponentManager(cfg, clusterName)
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

		manager := components.NewComponentManager(cfg, clusterName)
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

		manager := components.NewComponentManager(cfg, clusterName)
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

		manager := components.NewComponentManager(cfg, clusterName)
		fmt.Printf("%-20s %-10s %-10s %-15s %s\n", "NAME", "TYPE", "VERSION", "NAMESPACE", "STATUS")
		// Sort the components by name.
		names := make([]string, 0, len(cfg.Components))
		for name := range cfg.Components {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			component := cfg.Components[name]
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

		manager := components.NewComponentManager(cfg, clusterName)
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

// createDefaultConfigFile writes the embedded default component config to the
// user's rockpool directory. Unless force is true, an existing config file is
// left untouched so that local edits are never overwritten.
func createDefaultConfigFile(force bool) error {
	configPath := getDefaultConfigPath()
	if err := os.MkdirAll(getRockpoolDir(), 0755); err != nil {
		return fmt.Errorf("failed to create rockpool directory: %w", err)
	}

	if !force {
		if _, err := os.Stat(configPath); err == nil {
			log.Debug().Msgf("config file already exists at %s; leaving it as is", configPath)
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to check for existing config file: %w", err)
		}
	}

	log.Debug().Msgf("writing default config file at %s", configPath)

	cfg, err := components.CreateDefaultConfig()
	if err != nil {
		return fmt.Errorf("failed to create default config: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

// createTemplates copies the embedded templates to the user's rockpool
// directory. Unless force is true, templates which already exist are left
// untouched so that local edits are never overwritten.
func createTemplates(force bool) error {
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

		dest := filepath.Join(templatesDir, entry.Name())
		if !force {
			if _, err := os.Stat(dest); err == nil {
				log.Debug().Msgf("template already exists at %s; leaving it as is", dest)
				continue
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("failed to check for existing template %s: %w", entry.Name(), err)
			}
		}

		data, err := components.DefaultComponents.ReadFile(filepath.Join("default/templates", entry.Name()))
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", entry.Name(), err)
		}

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

	// Set the dnsIp in the config.
	if cfg.DnsIp == "" {
		cfg.DnsIp = dnsIp
	}

	// Set the lagoon version in the config.
	if cfg.LagoonVersion == "" {
		cfg.LagoonVersion = lagoonVersion
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

	componentGenerateConfigCmd.Flags().BoolVar(&forceGenerate, "force", false,
		"Overwrite existing config and templates with the embedded defaults, "+
			"discarding local modifications")

	// Add flags
	componentCmd.PersistentFlags().StringVar(&rockpoolDir, "rockpool-dir",
		getRockpoolDir(), "Path to the rockpool configuration directory")
	componentCmd.PersistentFlags().StringVarP(&configFile, "config", "c",
		getDefaultConfigPath(), "Path to the components configuration file")
	componentCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n",
		"", "Override the namespace for the component")
	componentCmd.PersistentFlags().StringVar(&clusterName, "cluster",
		"colima-rockpool", "Override the cluster for the component")
	componentCmd.PersistentFlags().StringVarP(&domain, "domain", "d",
		"local",
		`The base domain of the platform; ancillary services will be created as
its subdomains using the provided 'name', e.g, rockpool.local,
lagoon.rockpool.local`)
	componentCmd.PersistentFlags().StringVar(&dnsIp, "dns-ip", "127.0.0.1",
		"Override the DNS IP address for the component")
	componentCmd.PersistentFlags().StringVar(&lagoonVersion, "lagoon-version",
		"v2.25.0", "Set the Lagoon version for the platform")
}
