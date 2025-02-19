package config

type ComponentConfig struct {
	// Name of the component.
	Name string `yaml:"name"`
	// Version of the component to install.
	Version string `yaml:"version"`
	// Type of the component (helm, kubernetes, custom).
	Type string `yaml:"type"`
	// Enabled indicates if the component should be installed.
	Enabled bool `yaml:"enabled"`
	// Namespace to install the component in.
	Namespace string `yaml:"namespace,omitempty"`
	// Dependencies are components that must be installed before this one.
	Dependencies []string `yaml:"dependencies,omitempty"`
	// Chart specifies the helm chart to install (required when type is helm).
	Chart string `yaml:"chart,omitempty"`
	// Values for helm charts.
	Values map[string]interface{} `yaml:"values,omitempty"`
	// ValuesTemplate specifies a template file to load Helm values from.
	ValuesTemplate string `yaml:"valuesTemplate,omitempty"`
	// Paths to kubernetes manifests to install.
	ManifestPaths []string `yaml:"manifestPaths,omitempty"`
	// Hooks for running commands before or after installation/upgrade.
	Hooks ComponentHooks `yaml:"hooks,omitempty"`
}

type ComponentHooks struct {
	PreInstall  []string `yaml:"preInstall,omitempty"`
	PostInstall []string `yaml:"postInstall,omitempty"`
	PreUpgrade  []string `yaml:"preUpgrade,omitempty"`
	PostUpgrade []string `yaml:"postUpgrade,omitempty"`
}

type Config struct {
	// Dir is the user's local directory for configuration files.
	Dir string `yaml:"dir"`
	// Domain is the domain name of the cluster.
	Domain string `yaml:"domain"`
	// DnsIp is the IP address of the DNS server.
	DnsIp string `yaml:"dnsIp"`
	// LagoonVersion is the version of Lagoon to install.
	LagoonVersion string `yaml:"lagoonVersion"`
	// Components are the components to install.
	Components map[string]ComponentConfig `yaml:"components"`
}
