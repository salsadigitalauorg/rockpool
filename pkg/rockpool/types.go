package rockpool

import (
	"encoding/json"
	"sync"

	"github.com/briandowns/spinner"
)

type Registry struct {
	Name  string `json:"name"`
	State struct {
		Running bool
		Status  string
	}
}

type ClusterNode struct {
	Name  string `json:"name"`
	Role  string `json:"role"`
	State struct {
		Running bool
		Status  string
	}
	IP struct {
		IP string
	}
}

type Cluster struct {
	Name           string        `json:"name"`
	ServersRunning int           `json:"serversRunning"`
	ServersCount   int           `json:"serversCount"`
	AgentsRunning  int           `json:"agentsRunning"`
	AgentsCount    int           `json:"agentsCount"`
	Nodes          []ClusterNode `json:"nodes"`
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
	Spinner     spinner.Spinner
	Clusters    ClusterList
	Registry    Registry
	BinaryPaths map[string]string
	// List of Helm releases per cluster.
	HelmReleases map[string][]HelmRelease
	// Kubeconfig per cluster.
	Kubeconfig map[string]string
}

type Config struct {
	ClusterName           string
	Hostname              string
	LagoonBaseUrl         string
	HarborPass            string
	Arch                  string
	RenderedTemplatesPath string
	UpgradeComponents     []string
	NumTargets            int
}

type Rockpool struct {
	State
	Config
	wg *sync.WaitGroup
}

type DockerContainer struct {
	Name            string
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string
		}
	}
}

type CoreDNSConfigMap struct {
	ApiVersion string `json:"apiVersion"`
	Data       struct {
		Corefile  string
		NodeHosts string
	} `json:"data"`
	Kind     string          `json:"kind"`
	Metadata json.RawMessage `json:"metadata"`
}
