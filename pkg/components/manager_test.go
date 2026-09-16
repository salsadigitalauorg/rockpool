package components

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/salsadigitalauorg/rockpool/pkg/config"
)

func newTestManager() *ComponentManager {
	return NewComponentManager(&config.Config{
		Domain: "local",
	}, "rockpool")
}

// A failing hook must surface what the hook actually said, otherwise failures
// are reported as a bare exit status with no indication of the cause.
func TestRunHooksIncludesOutputOnFailure(t *testing.T) {
	m := newTestManager()

	err := m.runHooks(config.ComponentConfig{}, []string{"echo 'the useful diagnostic'; exit 3"})
	if err == nil {
		t.Fatal("expected an error from a failing hook, got nil")
	}
	if !strings.Contains(err.Error(), "the useful diagnostic") {
		t.Errorf("expected hook output to be included in the error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "hook failed") {
		t.Errorf("expected error to be labelled as a hook failure, got: %v", err)
	}
}

// stderr must be captured too, since that is where most tooling reports errors.
func TestRunHooksCapturesStderr(t *testing.T) {
	m := newTestManager()

	err := m.runHooks(config.ComponentConfig{}, []string{"echo 'written to stderr' >&2; exit 1"})
	if err == nil {
		t.Fatal("expected an error from a failing hook, got nil")
	}
	if !strings.Contains(err.Error(), "written to stderr") {
		t.Errorf("expected stderr to be captured in the error, got: %v", err)
	}
}

func TestRunHooksSucceedsForPassingHooks(t *testing.T) {
	m := newTestManager()

	if err := m.runHooks(config.ComponentConfig{}, []string{"true", "echo output is fine"}); err != nil {
		t.Errorf("expected passing hooks to succeed, got: %v", err)
	}
}

// Hooks run in order and must stop at the first failure, so that a later hook
// does not run against a half-configured component.
func TestRunHooksStopsAtFirstFailure(t *testing.T) {
	m := newTestManager()

	err := m.runHooks(config.ComponentConfig{}, []string{"exit 1", "echo should-not-run"})
	if err == nil {
		t.Fatal("expected an error from a failing hook, got nil")
	}
	if strings.Contains(err.Error(), "should-not-run") {
		t.Errorf("expected execution to stop at the first failure, got: %v", err)
	}
}

func TestRunHooksNoHooksIsNoOp(t *testing.T) {
	m := newTestManager()

	if err := m.runHooks(config.ComponentConfig{}, nil); err != nil {
		t.Errorf("expected no error for an empty hook list, got: %v", err)
	}
}

// Hooks are rendered through text/template before execution, so they can
// reference cluster-specific values like {{ .Hostname }} instead of
// hardcoding them.
func TestRunHooksRendersTemplateData(t *testing.T) {
	m := newTestManager()

	if err := m.runHooks(config.ComponentConfig{}, []string{
		`test "{{ .Hostname }}" = "rockpool.local"`,
	}); err != nil {
		t.Errorf("expected hook referencing .Hostname to render and succeed, got: %v", err)
	}
}

// NewTemplateData composes the hostname from the cluster name and domain; the
// templates rely on this to build service URLs.
func TestNewTemplateDataDerivesHostname(t *testing.T) {
	m := NewComponentManager(&config.Config{
		Domain:        "local",
		DnsIp:         "127.0.0.1",
		LagoonVersion: "v2.22.0",
	}, "rockpool")

	data := m.NewTemplateData(config.ComponentConfig{Name: "harbor"})

	if data.Hostname != "rockpool.local" {
		t.Errorf("expected hostname %q, got %q", "rockpool.local", data.Hostname)
	}
	if data.Name != "rockpool" {
		t.Errorf("expected name %q, got %q", "rockpool", data.Name)
	}
	if data.LagoonVersion != "v2.22.0" {
		t.Errorf("expected lagoon version %q, got %q", "v2.22.0", data.LagoonVersion)
	}
	if data.DnsIp != "127.0.0.1" {
		t.Errorf("expected dns ip %q, got %q", "127.0.0.1", data.DnsIp)
	}
	if data.Component.Name != "harbor" {
		t.Errorf("expected component to be carried through, got %q", data.Component.Name)
	}
}

// Values templates interpolate user-supplied strings such as the RabbitMQ
// password. Rendered unquoted, a value containing YAML metacharacters either
// breaks parsing or silently changes type, so 'quote' must always produce a
// scalar that round-trips back to the original string.
func TestQuoteProducesRoundTrippableYamlScalar(t *testing.T) {
	inputs := []string{
		"pass",
		"pa: ss",
		"[a,b]",
		"{a: b}",
		"*anchor",
		"&anchor",
		"pa#ss",
		"!!str weird",
		"100",
		"true",
		`has"double`,
		`has\backslash`,
		"",
	}

	for _, in := range inputs {
		doc := "rabbitMQPassword: " + quote(in) + "\n"

		var parsed struct {
			RabbitMQPassword string `yaml:"rabbitMQPassword"`
		}
		if err := yaml.Unmarshal([]byte(doc), &parsed); err != nil {
			t.Errorf("input %q rendered unparseable YAML %q: %v", in, doc, err)
			continue
		}
		if parsed.RabbitMQPassword != in {
			t.Errorf("input %q round-tripped as %q (rendered %q)",
				in, parsed.RabbitMQPassword, doc)
		}
	}
}

// The quote func must be registered on every rendering path, not just values
// files, so templates can rely on it uniformly.
func TestQuoteIsAvailableToHooksAndManifests(t *testing.T) {
	m := NewComponentManager(&config.Config{
		Domain:           "local",
		RabbitMQPassword: "pa: ss",
	}, "rockpool")

	rendered, err := m.renderHook(config.ComponentConfig{},
		`echo {{ .RabbitMQPassword | quote }}`)
	if err != nil {
		t.Fatalf("expected quote to be available to hooks, got: %v", err)
	}
	if !strings.Contains(rendered, `"pa: ss"`) {
		t.Errorf("expected quoted password in rendered hook, got %q", rendered)
	}
}
