package components

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/salsadigitalauorg/rockpool/pkg/config"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
)

// NewComponentManager creates a new component manager
func NewComponentManager(cfg *config.Config, clusterName string) *ComponentManager {
	return &ComponentManager{
		config:      cfg,
		clusterName: clusterName,
	}
}

// NewTemplateData creates a new TemplateData struct with default values
func (m *ComponentManager) NewTemplateData(component config.ComponentConfig) *TemplateData {
	return &TemplateData{
		Component:     component,
		Hostname:      m.clusterName + "." + m.config.Domain,
		Arch:          runtime.GOARCH,
		Name:          m.clusterName,
		LagoonVersion: m.config.LagoonVersion,
		Domain:        m.config.Domain,
		DnsIp:         m.config.DnsIp,
	}
}

func (m *ComponentManager) renderHelmValuesFile(component config.ComponentConfig) (string, error) {
	valuesTemplate := filepath.Clean(filepath.Join(m.config.Dir, component.ValuesTemplate))
	valuesBytes, err := os.ReadFile(valuesTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to read values template: %w", err)
	}

	// Create a new template and parse the content
	tmpl, err := template.New(filepath.Base(component.ValuesTemplate)).Parse(string(valuesBytes))
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", valuesTemplate, err)
	}

	// Create a buffer to store the rendered template
	var renderedBuf strings.Builder

	// Create template data with component values
	templateData := m.NewTemplateData(component)

	// Execute the template with the template data
	if err := tmpl.Execute(&renderedBuf, templateData); err != nil {
		return "", fmt.Errorf("failed to render template %s: %w", valuesTemplate, err)
	}

	return renderedBuf.String(), nil
}

func (m *ComponentManager) renderManifest(component config.ComponentConfig, manifestPath string) (string, error) {
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("failed to read manifest: %w", err)
	}

	// Create a new template and parse the content
	tmpl, err := template.New(filepath.Base(manifestPath)).Parse(string(manifest))
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", manifestPath, err)
	}

	// Create a buffer to store the rendered template
	var renderedBuf strings.Builder

	// Create template data with component values
	templateData := m.NewTemplateData(component)

	// Execute the template with the template data
	if err := tmpl.Execute(&renderedBuf, templateData); err != nil {
		return "", fmt.Errorf("failed to render template %s: %w", manifestPath, err)
	}

	return renderedBuf.String(), nil
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

		// Process values.
		for key, value := range component.Values {
			helmArgs = append(helmArgs, fmt.Sprintf("--set=%s=%v", key, value))
		}

		// Add version if specified
		if component.Version != "" {
			helmArgs = append(helmArgs, "--version", component.Version)
		}

		// Use the component name as release name and specified chart
		cmd := helm.Exec(m.clusterName, component.Namespace, "install", component.Name, component.Chart)
		cmd.AddArgs(helmArgs...)

		// Render the values file
		if component.ValuesTemplate != "" {
			renderedValues, err := m.renderHelmValuesFile(component)
			if err != nil {
				return fmt.Errorf("failed to render values: %w", err)
			}
			cmd.AddArgs("--values", "-")
			cmd.SetStdin(strings.NewReader(renderedValues))
		}

		err := cmd.RunProgressive()
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
			manifestPath := filepath.Clean(filepath.Join(m.config.Dir, path))

			// Render the manifest
			renderedManifest, err := m.renderManifest(component, manifestPath)
			if err != nil {
				return fmt.Errorf("failed to render manifest: %w", err)
			}

			// Apply the kubernetes manifests
			err = kube.ApplyInline(m.clusterName, component.Namespace, renderedManifest, false)
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
		cmd := helm.Exec(m.clusterName, component.Namespace, "upgrade", component.Name, component.Chart)
		cmd.AddArgs(helmArgs...)

		// Render the values file
		if component.ValuesTemplate != "" {
			renderedValues, err := m.renderHelmValuesFile(component)
			if err != nil {
				return fmt.Errorf("failed to render values: %w", err)
			}
			cmd.AddArgs("--values", "-")
			cmd.SetStdin(strings.NewReader(renderedValues))
		}

		err := cmd.RunProgressive()
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
			manifestPath := filepath.Clean(filepath.Join(m.config.Dir, path))

			// Render the manifest
			renderedManifest, err := m.renderManifest(component, manifestPath)
			if err != nil {
				return fmt.Errorf("failed to render manifest: %w", err)
			}

			// Apply the kubernetes manifests with force flag for upgrade
			err = kube.ApplyInline(m.clusterName, component.Namespace, renderedManifest, true)
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
			manifestPath := filepath.Clean(filepath.Join(m.config.Dir, path))

			// Render the manifest
			renderedManifest, err := m.renderManifest(component, manifestPath)
			if err != nil {
				return fmt.Errorf("failed to render manifest: %w", err)
			}

			// Use kubectl delete for kubernetes manifests
			cmd := kube.Cmd(m.clusterName, component.Namespace, "delete", "-f", "-")
			cmd.SetStdin(strings.NewReader(renderedManifest))
			err = cmd.RunProgressive()
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
			manifestPath := filepath.Clean(filepath.Join(m.config.Dir, path))

			// Render the manifest
			renderedManifest, err := m.renderManifest(component, manifestPath)
			if err != nil {
				return false, fmt.Errorf("failed to render manifest: %w", err)
			}

			cmd := kube.Cmd(m.clusterName, component.Namespace, "get", "-f", "-")
			cmd.SetStdin(strings.NewReader(renderedManifest))
			_, err = cmd.Output()
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
