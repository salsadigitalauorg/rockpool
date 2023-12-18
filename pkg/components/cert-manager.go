package components

import (
	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
)

func init() {
	Add("cert-manager", Component{
		Name: "cert-manager",
		InstallActions: []action.Action{
			kube.Applyer{
				Info:      "installing cert-manager",
				Namespace: "",
				Template:  "cert-manager.yaml",
				Force:     true,
			},
			kube.Waiter{
				Namespace: "cert-manager",
				Resource:  "deployment/cert-manager-webhook",
				Condition: "Available=true",
				Retries:   10,
				Delay:     5,
			},
			kube.Applyer{
				Namespace: "cert-manager",
				Template:  "ca.yml.tmpl",
				Force:     true,
				Retries:   30,
				Delay:     10,
			},
		},
	})
}
