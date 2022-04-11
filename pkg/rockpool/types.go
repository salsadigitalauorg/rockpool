package rockpool

type Cluster struct {
	Name string `json:"name"`
}

type HelmRelease struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Revision   string `json:"revision"`
	Updated    string `json:"updated"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"app_version"`
}

type State struct {
	BinaryPaths  map[string]string
	HelmReleases []HelmRelease
	Kubeconfig   string
}

type Config struct {
	ClusterName   string
	LagoonBaseUrl string
	HarborPass    string
}
