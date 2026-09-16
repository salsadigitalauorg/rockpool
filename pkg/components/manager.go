package components

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/salsadigitalauorg/rockpool/pkg/config"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"

	log "github.com/sirupsen/logrus"
)

// NewComponentManager creates a new component manager
func NewComponentManager(cfg *config.Config, clusterName string) *ComponentManager {
	return &ComponentManager{
		config:      cfg,
		clusterName: clusterName,
	}
}

// templateFuncs are made available to every values file, manifest, and hook
// rendered through text/template.
var templateFuncs = template.FuncMap{
	"trimPrefix": strings.TrimPrefix,
	"quote":      quote,
}

// quote renders v as a YAML double-quoted scalar. Interpolating a
// user-supplied value into a values template unquoted lets characters that are
// significant to YAML (':', '[', '{', '*', '#') either break parsing or
// silently change the value's type — a password of "[a,b]" would otherwise
// become a list. Go's escape syntax is a subset of YAML's double-quoted style,
// so strconv.Quote produces a valid scalar and never line-folds.
func quote(v interface{}) string {
	return strconv.Quote(fmt.Sprintf("%v", v))
}

// NewTemplateData creates a new TemplateData struct with default values
func (m *ComponentManager) NewTemplateData(component config.ComponentConfig) *TemplateData {
	return &TemplateData{
		Component:        component,
		Hostname:         m.clusterName + "." + m.config.Domain,
		Name:             m.clusterName,
		LagoonVersion:    m.config.LagoonVersion,
		Domain:           m.config.Domain,
		DnsIp:            m.config.DnsIp,
		RabbitMQPassword: m.config.RabbitMQPassword,
	}
}

func (m *ComponentManager) renderHelmValuesFile(component config.ComponentConfig) (string, error) {
	valuesTemplate := filepath.Clean(filepath.Join(m.config.Dir, component.ValuesTemplate))
	valuesBytes, err := os.ReadFile(valuesTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to read values template: %w", err)
	}

	// Create a new template and parse the content
	tmpl, err := template.New(filepath.Base(component.ValuesTemplate)).Funcs(templateFuncs).Parse(string(valuesBytes))
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
	tmpl, err := template.New(filepath.Base(manifestPath)).Funcs(templateFuncs).Parse(string(manifest))
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
	if err := kube.ValidateCluster(m.clusterName); err != nil {
		return fmt.Errorf("cluster validation failed: %w", err)
	}

	// Run pre-install hooks
	if err := m.runHooks(component, component.Hooks.PreInstall); err != nil {
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
	if err := m.runHooks(component, component.Hooks.PostInstall); err != nil {
		return fmt.Errorf("post-install hooks failed: %w", err)
	}

	return nil
}

func (m *ComponentManager) Upgrade(ctx context.Context, component config.ComponentConfig) error {
	if err := kube.ValidateCluster(m.clusterName); err != nil {
		return fmt.Errorf("cluster validation failed: %w", err)
	}

	// Run pre-upgrade hooks
	if err := m.runHooks(component, component.Hooks.PreUpgrade); err != nil {
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
	if err := m.runHooks(component, component.Hooks.PostUpgrade); err != nil {
		return fmt.Errorf("post-upgrade hooks failed: %w", err)
	}

	return nil
}

func (m *ComponentManager) Uninstall(ctx context.Context, component config.ComponentConfig) error {
	if err := kube.ValidateCluster(m.clusterName); err != nil {
		return fmt.Errorf("cluster validation failed: %w", err)
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
	if err := kube.ValidateCluster(m.clusterName); err != nil {
		return false, fmt.Errorf("cluster validation failed: %w", err)
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

// renderHook renders a hook string through text/template using the same
// TemplateData available to values files and manifests, so hooks can
// reference e.g. {{ .Hostname }} instead of hardcoding cluster-specific
// values.
//
// Note that this reserves '{{' inside hook bodies: a hook using
// 'kubectl -o go-template' must escape its own delimiters as {{"{{"}}.
// Single-brace forms such as -o jsonpath='{.items[0]}' and awk '{print $1}'
// are unaffected.
func (m *ComponentManager) renderHook(component config.ComponentConfig, hook string) (string, error) {
	tmpl, err := template.New("hook").Funcs(templateFuncs).Parse(hook)
	if err != nil {
		return "", fmt.Errorf("failed to parse hook template: %w", err)
	}

	var renderedBuf strings.Builder
	if err := tmpl.Execute(&renderedBuf, m.NewTemplateData(component)); err != nil {
		return "", fmt.Errorf("failed to render hook template: %w", err)
	}

	return renderedBuf.String(), nil
}

func (m *ComponentManager) runHooks(component config.ComponentConfig, hooks []string) error {
	for _, rawHook := range hooks {
		hook, err := m.renderHook(component, rawHook)
		if err != nil {
			return fmt.Errorf("failed to render hook: %w", err)
		}

		logger := log.WithField("hook", hook)
		logger.Debug("running hook")

		cmd := exec.Command("bash", "-c", hook)
		// Capture stdout and stderr together so that failures can report what
		// the hook actually said, rather than a bare exit status.
		output, err := cmd.CombinedOutput()
		trimmed := strings.TrimSpace(string(output))

		if err != nil {
			if trimmed != "" {
				logger.WithField("output", trimmed).Error("hook failed")
				return fmt.Errorf("hook failed: %s: %w: %s", hook, err, trimmed)
			}
			logger.Error("hook failed with no output")
			return fmt.Errorf("hook failed: %s: %w", hook, err)
		}

		if trimmed != "" {
			logger.WithField("output", trimmed).Debug("hook output")
		}
	}
	return nil
}
