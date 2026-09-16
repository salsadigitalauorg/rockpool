package components

import (
	"strings"
	"testing"

	"github.com/salsadigitalauorg/rockpool/pkg/config"
)

func TestParseComponentFileMultipleDocuments(t *testing.T) {
	data := []byte(`name: mariadb
type: kubernetes
namespace: mariadb

---
name: mariadb-production
type: kubernetes
namespace: mariadb

---
name: mariadb-development
type: kubernetes
namespace: mariadb
`)

	got := map[string]config.ComponentConfig{}
	if err := parseComponentFile("mariadb.yaml", data, got); err != nil {
		t.Fatalf("parseComponentFile() returned unexpected error: %v", err)
	}

	want := []string{"mariadb", "mariadb-production", "mariadb-development"}
	if len(got) != len(want) {
		t.Fatalf("expected %d components, got %d: %v", len(want), len(got), keys(got))
	}
	for _, name := range want {
		component, ok := got[name]
		if !ok {
			t.Errorf("expected component %q to be present, got %v", name, keys(got))
			continue
		}
		if component.Type != "kubernetes" {
			t.Errorf("component %q: expected type %q, got %q",
				name, "kubernetes", component.Type)
		}
		if component.Namespace != "mariadb" {
			t.Errorf("component %q: expected namespace %q, got %q",
				name, "mariadb", component.Namespace)
		}
	}
}

func TestParseComponentFileSingleDocument(t *testing.T) {
	data := []byte(`name: cert-manager
type: helm
chart: jetstack/cert-manager
`)

	got := map[string]config.ComponentConfig{}
	if err := parseComponentFile("cert-manager.yaml", data, got); err != nil {
		t.Fatalf("parseComponentFile() returned unexpected error: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 component, got %d: %v", len(got), keys(got))
	}
	if got["cert-manager"].Chart != "jetstack/cert-manager" {
		t.Errorf("expected chart %q, got %q",
			"jetstack/cert-manager", got["cert-manager"].Chart)
	}
}

// A file with no explicit name falls back to the filename, preserving the
// previous behaviour for single-document files.
func TestParseComponentFileNameDefaultsToFileName(t *testing.T) {
	data := []byte("type: helm\nchart: example/example\n")

	got := map[string]config.ComponentConfig{}
	if err := parseComponentFile("dnsmasq.yaml", data, got); err != nil {
		t.Fatalf("parseComponentFile() returned unexpected error: %v", err)
	}

	if _, ok := got["dnsmasq"]; !ok {
		t.Errorf("expected component to be named after the file, got %v", keys(got))
	}
}

// Only the first document can take the implicit filename, otherwise later
// documents would silently overwrite earlier ones.
func TestParseComponentFileUnnamedLaterDocumentErrors(t *testing.T) {
	data := []byte("name: first\ntype: helm\n---\ntype: helm\n")

	err := parseComponentFile("multi.yaml", data, map[string]config.ComponentConfig{})
	if err == nil {
		t.Fatal("expected an error for an unnamed second document, got nil")
	}
	if !strings.Contains(err.Error(), "document 2 has no name") {
		t.Errorf("expected error to identify the offending document, got: %v", err)
	}
}

func TestParseComponentFileDuplicateNameErrors(t *testing.T) {
	data := []byte("name: dupe\ntype: helm\n---\nname: dupe\ntype: kubernetes\n")

	err := parseComponentFile("dupe.yaml", data, map[string]config.ComponentConfig{})
	if err == nil {
		t.Fatal("expected an error for a duplicate component name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate component name") {
		t.Errorf("expected a duplicate-name error, got: %v", err)
	}
}

func TestParseComponentFileInvalidYamlErrors(t *testing.T) {
	data := []byte("name: broken\n\ttype: helm\n")

	err := parseComponentFile("broken.yaml", data, map[string]config.ComponentConfig{})
	if err == nil {
		t.Fatal("expected an error for malformed YAML, got nil")
	}
}

// CreateDefaultConfig reads the real embedded defaults, so this guards the
// actual shipped component files rather than a fixture.
func TestCreateDefaultConfigLoadsEmbeddedComponents(t *testing.T) {
	cfg, err := CreateDefaultConfig()
	if err != nil {
		t.Fatalf("CreateDefaultConfig() returned unexpected error: %v", err)
	}

	// mariadb.yaml is a multi-document file; all three variants must load.
	// lagoon-remote-values.yml.tmpl depends on the production and development
	// services existing.
	for _, name := range []string{"mariadb", "mariadb-production", "mariadb-development"} {
		if _, ok := cfg.Components[name]; !ok {
			t.Errorf("expected embedded component %q to be loaded", name)
		}
	}

	// Spot-check a representative single-document component.
	if _, ok := cfg.Components["cert-manager"]; !ok {
		t.Error(`expected embedded component "cert-manager" to be loaded`)
	}

	for name, component := range cfg.Components {
		if component.Type == "" {
			t.Errorf("embedded component %q has no type", name)
		}
		if component.Name != name {
			t.Errorf("embedded component keyed as %q has mismatched name %q",
				name, component.Name)
		}
	}
}

func keys(m map[string]config.ComponentConfig) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
