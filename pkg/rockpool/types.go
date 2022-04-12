package rockpool

import "sync"

type Cluster struct {
	Name           string `json:"name"`
	ServersRunning int    `json:"serversRunning"`
	ServersCount   int    `json:"serversCount"`
	AgentsRunning  int    `json:"agentsRunning"`
	AgentsCount    int    `json:"agentsCount"`
}

type ClusterList []Cluster

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
	Clusters    ClusterList
	BinaryPaths map[string]string
	// List of Helm releases per cluster.
	HelmReleases map[string][]HelmRelease
	// Kubeconfig per cluster.
	Kubeconfig map[string]string
}

type Config struct {
	ClusterName           string
	LagoonBaseUrl         string
	HarborPass            string
	Arch                  string
	RenderedTemplatesPath string
	UpgradeComponents     []string
}

type Rockpool struct {
	State
	Config
	wg *sync.WaitGroup
}
