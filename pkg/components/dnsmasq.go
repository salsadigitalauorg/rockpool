package components

import (
	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
)

func init() {
	Add("dnsmasq", Component{
		Name: "dnsmasq",
		InstallActions: []action.Action{
			kube.Applyer{
				Info:      "installing dnsmasq",
				Namespace: "default",
				Force:     true,
				Template:  "dnsmasq.yml.tmpl",
			},
		},
	})
}
