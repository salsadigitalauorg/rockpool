package rockpool

import (
	"encoding/json"

	"github.com/briandowns/spinner"
	"github.com/salsadigitalauorg/rockpool/pkg/k3d"
	"github.com/shurcooL/graphql"
	"golang.org/x/sync/syncmap"
)

type Remote struct {
	Id            int    `json:"id"`
	Name          string `json:"name"`
	ConsoleUrl    string `json:"consoleUrl"`
	RouterPattern string `json:"routerPattern"`
}

type State struct {
	Spinner  spinner.Spinner
	Registry k3d.Registry
	// Use syncmap.Map instead of a regular map for the following so there's no
	// race conditions during concurrent runs, which was happening before.
	// See https://stackoverflow.com/a/45585833/351590.
	// List of Helm releases per cluster.
	HelmReleases         syncmap.Map
	Remotes              []Remote
	HarborSecretManifest string
	HarborCaCrtFile      string
}

type Config struct {
	ConfigDir         string
	Name              string
	Domain            string
	Arch              string
	UpgradeComponents []string
	NumTargets        int
	LagoonSshKey      string
}

type Rockpool struct {
	State
	Config
	GqlClient *graphql.Client
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
