package docker

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

type Container struct {
	Name            string
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string
		}
	}
}
