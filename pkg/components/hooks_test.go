package components

import (
	"regexp"
	"strings"
	"testing"
)

// Install uses the component name as the Helm release name, so every resource
// the chart creates is prefixed with it. A hook that references a workload not
// carrying that prefix is left over from an earlier release name and will fail
// with "no matching resources found" on every install.
//
// Exceptions are charts that override the fullname, plus components installing
// into a namespace they do not own.
func TestHelmHookWorkloadReferencesUseTheReleaseNamePrefix(t *testing.T) {
	// Charts whose resource names are deliberately not release-prefixed.
	exempt := map[string]string{
		"cert-manager":   "chart sets fullnameOverride to cert-manager",
		"metrics-server": "chart sets fullnameOverride to metrics-server",
		"gitea":          "chart names its deployment after the release only",
	}

	workloadRef := regexp.MustCompile(
		`(?:deployment|deploy|statefulset|sts|daemonset|ds)/([a-z0-9][a-z0-9.-]*)`)

	cfg, err := CreateDefaultConfig()
	if err != nil {
		t.Fatalf("failed to load embedded components: %v", err)
	}

	for name, component := range cfg.Components {
		if component.Type != "helm" {
			continue
		}
		if reason, ok := exempt[name]; ok {
			t.Logf("skipping %s: %s", name, reason)
			continue
		}

		hooks := append([]string{}, component.Hooks.PreInstall...)
		hooks = append(hooks, component.Hooks.PostInstall...)
		hooks = append(hooks, component.Hooks.PreUpgrade...)
		hooks = append(hooks, component.Hooks.PostUpgrade...)

		for _, hook := range hooks {
			for _, match := range workloadRef.FindAllStringSubmatch(hook, -1) {
				resource := match[1]
				if !strings.HasPrefix(resource, name) {
					t.Errorf("component %q references workload %q, which does not "+
						"start with the release name %q; the chart will not create it",
						name, resource, name)
				}
			}
		}
	}
}

// 'kubectl wait --for=condition=Available' only has meaning for a Deployment.
// Applied to a StatefulSet it fails, so StatefulSets must use rollout status.
func TestHooksDoNotWaitForAvailableOnNonDeployments(t *testing.T) {
	cfg, err := CreateDefaultConfig()
	if err != nil {
		t.Fatalf("failed to load embedded components: %v", err)
	}

	for name, component := range cfg.Components {
		hooks := append([]string{}, component.Hooks.PostInstall...)
		hooks = append(hooks, component.Hooks.PostUpgrade...)

		for _, hook := range hooks {
			if !strings.Contains(hook, "condition=Available") {
				continue
			}
			for _, kind := range []string{"statefulset/", "sts/", "daemonset/", "ds/"} {
				if strings.Contains(hook, kind) {
					t.Errorf("component %q waits for condition=Available on a %s, "+
						"which never reports that condition; use 'kubectl rollout status'",
						name, strings.TrimSuffix(kind, "/"))
				}
			}
		}
	}
}
