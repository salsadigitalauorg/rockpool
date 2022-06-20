package rockpool

import (
	"encoding/json"

	"github.com/briandowns/spinner"
)

type Rockpool struct {
	Spinner spinner.Spinner
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
