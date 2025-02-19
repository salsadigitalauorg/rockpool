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
	"gopkg.in/yaml.v3"
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

// processValuesTemplate reads and processes a values template file, returning the parsed values
func (m *ComponentManager) processValuesTemplate(component config.ComponentConfig) (map[string]interface{}, error) {
	if component.ValuesTemplate == "" {
		return component.Values, nil
	}

	// Read the template file
	tmplContent, err := DefaultComponents.ReadFile(component.ValuesTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to read values template %s: %w", component.ValuesTemplate, err)
	}

	// Create a new template and parse the content
	tmpl, err := template.New(filepath.Base(component.ValuesTemplate)).Parse(string(tmplContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", component.ValuesTemplate, err)
	}

	// Create a buffer to store the rendered template
	var renderedBuf strings.Builder

	// Create template data with component values
	templateData := m.NewTemplateData(component)

	// Execute the template with the template data
	if err := tmpl.Execute(&renderedBuf, templateData); err != nil {
		return nil, fmt.Errorf("failed to render template %s: %w", component.ValuesTemplate, err)
	}

	// Parse the rendered YAML into a map
	var values map[string]interface{}
	if err := yaml.Unmarshal([]byte(renderedBuf.String()), &values); err != nil {
		return nil, fmt.Errorf("failed to parse rendered values: %w", err)
	}

	// If there are additional values specified, merge them with the template values
	// Values from component.Values take precedence
	for k, v := range component.Values {
		values[k] = v
	}

	return values, nil
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

		// Process values from template and/or direct values
		values, err := m.processValuesTemplate(component)
		if err != nil {
			return fmt.Errorf("failed to process values: %w", err)
		}

		// Convert values to helm arguments
		var helmArgs []string
		for key, value := range values {
			helmArgs = append(helmArgs, fmt.Sprintf("--set=%s=%v", key, value))
		}

		// Add version if specified
		if component.Version != "" {
			helmArgs = append(helmArgs, "--version", component.Version)
		}

		// Use the component name as release name and specified chart
		cmd := helm.Exec(m.clusterName, component.Namespace, "install", component.Name, component.Chart)
		cmd.AddArgs(helmArgs...)
		err = cmd.RunProgressive()
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

		// Process values from template and/or direct values
		values, err := m.processValuesTemplate(component)
		if err != nil {
			return fmt.Errorf("failed to process values: %w", err)
		}

		// Convert values to helm arguments
		var helmArgs []string
		for key, value := range values {
			helmArgs = append(helmArgs, fmt.Sprintf("--set=%s=%v", key, value))
		}

		// Add version if specified
		if component.Version != "" {
			helmArgs = append(helmArgs, "--version", component.Version)
		}

		// Use the component name as release name and specified chart
		cmd := helm.Exec(m.clusterName, component.Namespace, "upgrade", component.Name, component.Chart)
		cmd.AddArgs(helmArgs...)
		err = cmd.RunProgressive()
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
