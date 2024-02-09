package components

import (
	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
)

func init() {
	Add("dnsmasq", func() Component {
		return Component{
			Name:     "dnsmasq",
			CompType: ComponentTypeLocalReq,
			InstallActions: []action.Action{
				kube.Applyer{
					Info:      "installing dnsmasq",
					Namespace: "default",
					Force:     true,
					Template:  "dnsmasq.yml.tmpl",
				},
			},
		}
	})
}
