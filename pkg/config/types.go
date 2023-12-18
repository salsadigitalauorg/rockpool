package config

type ClusterProvider string

const (
	ClusterProviderColima ClusterProvider = "colima"
	ClusterProviderK3d    ClusterProvider = "k3d"
	ClusterProviderKind   ClusterProvider = "kind"
)

type ClusterConfig struct {
	// Path to the kubeconfig file for the cluster
	// (defaults to $HOME/.kube/config).
	Kubeconfig string `yaml:"kubeconfig"`

	// Name of the cluster context to use.
	// If not set, the current context will be used.
	Context string `yaml:"context"`
}

type Clusters struct {
	// Set the desired Kubernetes version for the cluster.
	// When using existing clusters, this will be used to check whether the
	// cluster is running the desired version.
	KubernetesVersion string `yaml:"kubernetes-version"`

	// Install all components on a single cluster.
	Single bool `yaml:"single"`

	// When set, the specified provider will be used to create the cluster
	// configuration.
	Provider ClusterProvider `yaml:"provider"`

	// When set, the specified cluster will be used to install Lagoon core.
	Core ClusterConfig `yaml:"core"`

	// When set, the specified clusters will be used to install Lagoon remotes.
	// Ignore if single is set to true.
	Remotes []ClusterConfig `yaml:"remotes"`
}

type HelmChartValues interface{}

type ComponentConfig struct {
	ChartValues HelmChartValues `yaml:"chart-values"`
}

type Config struct {
	// Name of the Rockpool instance.
	Name string `yaml:"name"`

	// The base domain of the platform; ancillary services will be created as
	// its subdomains using the provided 'name', e.g, rockpool.local,
	// lagoon.rockpool.local
	Domain string `yaml:"domain"`

	// Defines the clusters that Rockpool will use to install Lagoon on.
	Clusters `yaml:"clusters"`

	// Defines configuration for the components in the Lagoon installation.
	Components map[string]ComponentConfig `yaml:"components"`
}
