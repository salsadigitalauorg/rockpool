package k3d

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

type Registry struct {
	Name  string `json:"name"`
	State struct {
		Running bool
		Status  string
	}
}
