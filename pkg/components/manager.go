package components

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/salsadigitalauorg/rockpool/pkg/config"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
)

// Manager handles component lifecycle operations
type Manager interface {
	// Install installs a component
	Install(ctx context.Context, component config.ComponentConfig) error
	// Upgrade upgrades a component
	Upgrade(ctx context.Context, component config.ComponentConfig) error
	// Uninstall removes a component
	Uninstall(ctx context.Context, component config.ComponentConfig) error
	// IsInstalled checks if a component is installed
	IsInstalled(ctx context.Context, component config.ComponentConfig) (bool, error)
}

// ComponentManager implements the Manager interface
type ComponentManager struct {
	config      *config.Config
	clusterName string
}

// NewComponentManager creates a new component manager
func NewComponentManager(cfg *config.Config, clusterName string) *ComponentManager {
	return &ComponentManager{
		config:      cfg,
		clusterName: clusterName,
	}
}

func (m *ComponentManager) Install(ctx context.Context, component config.ComponentConfig) error {
	if !kube.ValidateCluster(m.clusterName) {
		return fmt.Errorf("cluster %s is not valid", m.clusterName)
	}

	// Run pre-install hooks
	if err := m.runHooks(component.Hooks.PreInstall); err != nil {
		return fmt.Errorf("pre-install hooks failed: %w", err)
	}

	// Install based on component type
	switch component.Type {
	case "helm":
		if component.Chart == "" {
			return fmt.Errorf("chart is required for helm components")
		}

		// Convert values to helm arguments
		var helmArgs []string
		for key, value := range component.Values {
			helmArgs = append(helmArgs, fmt.Sprintf("--set=%s=%v", key, value))
		}

		// Add version if specified
		if component.Version != "" {
			helmArgs = append(helmArgs, "--version", component.Version)
		}

		// Use the component name as release name and specified chart
		err := helm.InstallOrUpgrade(m.clusterName, component.Namespace, component.Name, component.Chart, helmArgs)
		if err != nil {
			return fmt.Errorf("helm installation failed: %w", err)
		}
	case "kubernetes":
		if len(component.ManifestPaths) == 0 {
			return fmt.Errorf("at least one manifest path is required for kubernetes components")
		}

		// Apply each manifest in order
		for _, path := range component.ManifestPaths {
			// Resolve the manifest path
			manifestPath := filepath.Clean(path)

			// Apply the kubernetes manifests
			err := kube.Apply(m.clusterName, component.Namespace, manifestPath, false)
			if err != nil {
				return fmt.Errorf("kubernetes manifest application failed for %s: %w", path, err)
			}
		}
	case "custom":
		// Custom installation logic
	default:
		return fmt.Errorf("unsupported component type: %s", component.Type)
	}

	// Run post-install hooks
	if err := m.runHooks(component.Hooks.PostInstall); err != nil {
		return fmt.Errorf("post-install hooks failed: %w", err)
	}

	return nil
}

func (m *ComponentManager) Upgrade(ctx context.Context, component config.ComponentConfig) error {
	if !kube.ValidateCluster(m.clusterName) {
		return fmt.Errorf("cluster %s is not valid", m.clusterName)
	}

	// Run pre-upgrade hooks
	if err := m.runHooks(component.Hooks.PreUpgrade); err != nil {
		return fmt.Errorf("pre-upgrade hooks failed: %w", err)
	}

	// Upgrade based on component type
	switch component.Type {
	case "helm":
		if component.Chart == "" {
			return fmt.Errorf("chart is required for helm components")
		}

		// Convert values to helm arguments
		var helmArgs []string
		for key, value := range component.Values {
			helmArgs = append(helmArgs, fmt.Sprintf("--set=%s=%v", key, value))
		}

		// Add version if specified
		if component.Version != "" {
			helmArgs = append(helmArgs, "--version", component.Version)
		}

		// Use the component name as release name and specified chart
		err := helm.InstallOrUpgrade(m.clusterName, component.Namespace, component.Name, component.Chart, helmArgs)
		if err != nil {
			return fmt.Errorf("helm upgrade failed: %w", err)
		}
	case "kubernetes":
		if len(component.ManifestPaths) == 0 {
			return fmt.Errorf("at least one manifest path is required for kubernetes components")
		}

		// Apply each manifest in order
		for _, path := range component.ManifestPaths {
			// Resolve the manifest path
			manifestPath := filepath.Clean(path)

			// Apply the kubernetes manifests with force flag for upgrade
			err := kube.Apply(m.clusterName, component.Namespace, manifestPath, true)
			if err != nil {
				return fmt.Errorf("kubernetes manifest upgrade failed for %s: %w", path, err)
			}
		}
	case "custom":
		// Custom upgrade logic
	default:
		return fmt.Errorf("unsupported component type: %s", component.Type)
	}

	// Run post-upgrade hooks
	if err := m.runHooks(component.Hooks.PostUpgrade); err != nil {
		return fmt.Errorf("post-upgrade hooks failed: %w", err)
	}

	return nil
}

func (m *ComponentManager) Uninstall(ctx context.Context, component config.ComponentConfig) error {
	if !kube.ValidateCluster(m.clusterName) {
		return fmt.Errorf("cluster %s is not valid", m.clusterName)
	}

	switch component.Type {
	case "helm":
		// Use helm uninstall command
		err := helm.Exec(m.clusterName, component.Namespace, "uninstall", component.Name).RunProgressive()
		if err != nil {
			return fmt.Errorf("helm uninstall failed: %w", err)
		}
	case "kubernetes":
		if len(component.ManifestPaths) == 0 {
			return fmt.Errorf("at least one manifest path is required for kubernetes components")
		}

		// Delete each manifest in reverse order
		for i := len(component.ManifestPaths) - 1; i >= 0; i-- {
			path := component.ManifestPaths[i]
			// Resolve the manifest path
			manifestPath := filepath.Clean(path)

			// Use kubectl delete for kubernetes manifests
			err := kube.Cmd(m.clusterName, component.Namespace, "delete", "-f", manifestPath).RunProgressive()
			if err != nil {
				return fmt.Errorf("kubernetes manifest deletion failed for %s: %w", path, err)
			}
		}
	case "custom":
		// Custom uninstall logic
	default:
		return fmt.Errorf("unsupported component type: %s", component.Type)
	}

	return nil
}

func (m *ComponentManager) IsInstalled(ctx context.Context, component config.ComponentConfig) (bool, error) {
	if !kube.ValidateCluster(m.clusterName) {
		return false, fmt.Errorf("cluster %s is not valid", m.clusterName)
	}

	switch component.Type {
	case "helm":
		// Fetch current releases
		helm.FetchInstalledReleases(m.clusterName)

		// Check if the release exists
		releases := helm.GetReleases(m.clusterName)
		for _, release := range releases {
			if release.Name == component.Name {
				return true, nil
			}
		}
		return false, nil

	case "kubernetes":
		if len(component.ManifestPaths) == 0 {
			return false, fmt.Errorf("at least one manifest path is required for kubernetes components")
		}

		// Check if all manifests exist
		for _, path := range component.ManifestPaths {
			manifestPath := filepath.Clean(path)
			_, err := kube.Cmd(m.clusterName, component.Namespace, "get", "-f", manifestPath).Output()
			if err != nil {
				return false, nil
			}
		}
		return true, nil

	case "custom":
		// For custom components, we assume it's installed if no error from the check command
		return true, nil

	default:
		return false, fmt.Errorf("unsupported component type: %s", component.Type)
	}
}

func (m *ComponentManager) runHooks(hooks []string) error {
	for _, hook := range hooks {
		cmd := exec.Command("sh", "-c", hook)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook failed: %s: %w", hook, err)
		}
	}
	return nil
}
