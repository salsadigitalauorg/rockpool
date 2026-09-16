package components

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/salsadigitalauorg/rockpool/pkg/config"
)

// renderEmbeddedManifest copies an embedded template into a temp dir and
// renders it for the given component, mirroring how Install resolves manifest
// paths relative to config.Dir.
func renderEmbeddedManifest(t *testing.T, templateName, componentName string) string {
	t.Helper()

	dir := t.TempDir()
	templatesDir := filepath.Join(dir, "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("failed to create templates dir: %v", err)
	}

	data, err := DefaultComponents.ReadFile("default/templates/" + templateName)
	if err != nil {
		t.Fatalf("failed to read embedded template %s: %v", templateName, err)
	}
	manifestPath := filepath.Join(templatesDir, templateName)
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatalf("failed to write template %s: %v", templateName, err)
	}

	m := NewComponentManager(&config.Config{Dir: dir, Domain: "local"}, "rockpool")

	rendered, err := m.renderManifest(config.ComponentConfig{Name: componentName}, manifestPath)
	if err != nil {
		t.Fatalf("component %q: renderManifest() returned unexpected error: %v",
			componentName, err)
	}
	return rendered
}

func containsTrimmedLine(rendered, want string) bool {
	for _, line := range strings.Split(rendered, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

// mariadb.yaml defines three variants (mariadb, mariadb-production,
// mariadb-development) that share a single manifest template. Each variant
// must render a distinct service name, since lagoon-remote-values.yml.tmpl
// depends on production.mariadb.svc.cluster.local and
// development.mariadb.svc.cluster.local existing as separate services.
func TestMariadbTemplateRendersDistinctServiceNamePerVariant(t *testing.T) {
	cases := []struct {
		componentName   string
		wantServiceName string
	}{
		{"mariadb", "mariadb"},
		{"mariadb-production", "production"},
		{"mariadb-development", "development"},
	}

	for _, c := range cases {
		rendered := renderEmbeddedManifest(t, "mariadb.yml.tmpl", c.componentName)

		wantLine := "name: " + c.wantServiceName
		if !containsTrimmedLine(rendered, wantLine) {
			t.Errorf("component %q: expected rendered manifest to contain %q, got:\n%s",
				c.componentName, wantLine, rendered)
		}
	}
}

// The mariadb entrypoint refuses to initialise without a root password option
// and the container exits, so a manifest with no MARIADB_ROOT_PASSWORD can
// never become ready. dbaas-operator connects as root using the password in
// lagoon-remote-values.yml.tmpl, so the two must agree.
func TestMariadbTemplateSetsRootPasswordMatchingDbaasOperator(t *testing.T) {
	remoteValues, err := DefaultComponents.ReadFile(
		"default/templates/lagoon-remote-values.yml.tmpl")
	if err != nil {
		t.Fatalf("failed to read embedded lagoon-remote values: %v", err)
	}
	if !strings.Contains(string(remoteValues), "password: mariadbpass") {
		t.Fatal("lagoon-remote-values.yml.tmpl no longer uses 'mariadbpass'; " +
			"update the mariadb manifest's MARIADB_ROOT_PASSWORD to match")
	}

	for _, name := range []string{"mariadb", "mariadb-production", "mariadb-development"} {
		rendered := renderEmbeddedManifest(t, "mariadb.yml.tmpl", name)

		if !containsTrimmedLine(rendered, "- name: MARIADB_ROOT_PASSWORD") {
			t.Errorf("component %q: manifest sets no MARIADB_ROOT_PASSWORD, so the "+
				"container will exit on start; got:\n%s", name, rendered)
			continue
		}
		if !containsTrimmedLine(rendered, "value: mariadbpass") {
			t.Errorf("component %q: MARIADB_ROOT_PASSWORD must be 'mariadbpass' to "+
				"match dbaas-operator; got:\n%s", name, rendered)
		}
	}
}

// An unpinned ':latest' tag makes environments non-reproducible and lets a
// major version land without warning.
func TestMariadbTemplatePinsImageTag(t *testing.T) {
	rendered := renderEmbeddedManifest(t, "mariadb.yml.tmpl", "mariadb")

	for _, line := range strings.Split(rendered, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "image:") {
			continue
		}
		image := strings.TrimSpace(strings.TrimPrefix(trimmed, "image:"))
		if strings.HasSuffix(image, ":latest") || !strings.Contains(image, ":") {
			t.Errorf("image %q is not pinned to an explicit version", image)
		}
		return
	}
	t.Error("no image line found in rendered mariadb manifest")
}

// mariadb data must survive a pod restart, otherwise every mariadb-backed
// database (including lagoon-core's own via dbaas-operator) is wiped on any
// reschedule. Each variant needs its own PersistentVolumeClaim — sharing one
// ReadWriteOnce claim across mariadb/production/development would make the
// second and third pods unschedulable — and the Deployment must mount it at
// /var/lib/mysql, where the image persists its data directory.
func TestMariadbTemplateProvisionsDistinctPersistentVolumePerVariant(t *testing.T) {
	for _, name := range []string{"mariadb", "mariadb-production", "mariadb-development"} {
		rendered := renderEmbeddedManifest(t, "mariadb.yml.tmpl", name)

		docs := decodeYamlDocs(t, name, rendered)

		var pvcName string
		var sawPVC bool
		for _, d := range docs {
			if kind(d) != "PersistentVolumeClaim" {
				continue
			}
			sawPVC = true
			pvcName = metaName(d)
			if pvcName != name {
				t.Errorf("%s: PersistentVolumeClaim name = %q, want %q (must be "+
					"unique per variant, not shared)", name, pvcName, name)
			}
			accessModes, _ := d["spec"].(map[string]interface{})["accessModes"].([]interface{})
			if len(accessModes) != 1 || accessModes[0] != "ReadWriteOnce" {
				t.Errorf("%s: expected accessModes [ReadWriteOnce], got %v", name, accessModes)
			}
		}
		if !sawPVC {
			t.Fatalf("%s: no PersistentVolumeClaim in rendered manifest:\n%s", name, rendered)
		}

		dep := findDoc(docs, "Deployment")
		if dep == nil {
			t.Fatalf("%s: no Deployment in rendered manifest", name)
		}

		if strategy, ok := dep["spec"].(map[string]interface{})["strategy"].(map[string]interface{}); !ok || strategy["type"] != "Recreate" {
			t.Errorf("%s: Deployment strategy must be Recreate for a ReadWriteOnce "+
				"volume, got %v", name, dep["spec"].(map[string]interface{})["strategy"])
		}

		podSpec := dep["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})

		volumes, _ := podSpec["volumes"].([]interface{})
		var claimName string
		for _, v := range volumes {
			vm := v.(map[string]interface{})
			if pvc, ok := vm["persistentVolumeClaim"].(map[string]interface{}); ok {
				claimName = pvc["claimName"].(string)
			}
		}
		if claimName != pvcName {
			t.Errorf("%s: Deployment volume claimName = %q, want %q", name, claimName, pvcName)
		}

		containers, _ := podSpec["containers"].([]interface{})
		if len(containers) == 0 {
			t.Fatalf("%s: Deployment has no containers", name)
		}
		mounts, _ := containers[0].(map[string]interface{})["volumeMounts"].([]interface{})
		var mountsDataDir bool
		for _, m := range mounts {
			mm := m.(map[string]interface{})
			if mm["mountPath"] == "/var/lib/mysql" {
				mountsDataDir = true
			}
		}
		if !mountsDataDir {
			t.Errorf("%s: container does not mount a volume at /var/lib/mysql, so "+
				"data does not survive a restart", name)
		}
	}
}

func decodeYamlDocs(t *testing.T, componentName, rendered string) []map[string]interface{} {
	t.Helper()
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(rendered)))
	var docs []map[string]interface{}
	for {
		var doc map[string]interface{}
		err := decoder.Decode(&doc)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("%s: rendered manifest is not valid YAML: %v", componentName, err)
		}
		docs = append(docs, doc)
	}
	return docs
}

func kind(doc map[string]interface{}) string {
	k, _ := doc["kind"].(string)
	return k
}

func metaName(doc map[string]interface{}) string {
	meta, _ := doc["metadata"].(map[string]interface{})
	name, _ := meta["name"].(string)
	return name
}

func findDoc(docs []map[string]interface{}, wantKind string) map[string]interface{} {
	for _, d := range docs {
		if kind(d) == wantKind {
			return d
		}
	}
	return nil
}
