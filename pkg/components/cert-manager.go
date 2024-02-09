package components

import (
	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
)

func init() {
	Add("cert-manager", Component{
		Name:     "cert-manager",
		CompType: ComponentTypeLagoonReq,
		InstallActions: []action.Action{
			helm.Installer{
				Info: "installing cert-manager",
				AddRepo: helm.HelmRepo{
					Name: "jetstack",
					Url:  "https://charts.jetstack.io",
				},
				Namespace:   "cert-manager",
				ReleaseName: "cert-manager",
				Chart:       "jetstack/cert-manager",
				Args: []string{
					"--set", "installCRDs=true",
					"--create-namespace", "--wait"},
			},
			// kube.Waiter{
			// 	Namespace: "cert-manager",
			// 	Resource:  "deployment/cert-manager-webhook",
			// 	Condition: "Available=true",
			// 	Retries:   10,
			// 	Delay:     5,
			// },
		},
	})
	Add("cert-manager-local-ca", Component{
		Name:     "cert-manager-local-ca",
		CompType: ComponentTypeLocalReq,
		InstallActions: []action.Action{
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
