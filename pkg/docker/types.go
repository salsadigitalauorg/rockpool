package docker

type Provider string

const (
	ProviderDockerDesktop  Provider = "docker-desktop"
	ProviderColima         Provider = "colima"
	ProviderRancherDesktop Provider = "rancher-desktop"
)

type DockerVersion struct {
	Client struct {
		Version string
		Context string
	}
	Server struct {
		Platform struct {
			Name string
		}
	}
}

type Context struct {
	Name           string
	Description    string
	Current        bool
	DockerEndpoint string
}

type ColimaProfile struct {
	Name    string
	Address string
}

type PsContainer struct {
	ID     string
	Names  string
	Labels string
	State  string
}

type Container struct {
	Name  string
	State struct {
		Status string
	}
	Config struct {
		Labels map[string]string
	}
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string
		}
	}
}
