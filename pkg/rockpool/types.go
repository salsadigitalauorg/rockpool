package rockpool

import (
	"encoding/json"

	"github.com/briandowns/spinner"
	"github.com/salsadigitalauorg/rockpool/pkg/k3d"
	"github.com/shurcooL/graphql"
)

type Remote struct {
	Id            int    `json:"id"`
	Name          string `json:"name"`
	ConsoleUrl    string `json:"consoleUrl"`
	RouterPattern string `json:"routerPattern"`
}

type State struct {
	Spinner              spinner.Spinner
	Registry             k3d.Registry
	Remotes              []Remote
	HarborSecretManifest string
	HarborCaCrtFile      string
}

type Rockpool struct {
	State
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
