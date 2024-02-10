package components

import (
	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
)

func init() {
	Add("ingress-nginx", func() Component {
		return Component{
			Name:     "ingress-nginx",
			CompType: ComponentTypeLagoonReq,
			InstallActions: []action.Action{
				helm.Installer{
					Info: "installing ingress-nginx",
					AddRepo: helm.HelmRepo{
						Name: "ingress-nginx",
						Url:  "https://kubernetes.github.io/ingress-nginx",
					},
					Namespace:   "ingress-nginx",
					ReleaseName: "ingress-nginx",
					Chart:       "ingress-nginx/ingress-nginx",
					Args: []string{
						"--create-namespace", "--wait",
						"--set", "controller.config.ssl-redirect=false",
						"--set", "controller.config.proxy-body-size=8m",
						"--set", "controller.ingressClassResource.default=true",
						"--set", "controller.service.type=NodePort",
						"--set", "controller.service.nodePorts.http=31080",
						"--set", "controller.service.nodePorts.https=31443",
						"--set", "controller.watchIngressWithoutClass=true",
						"--set", "server-name-hash-bucket-size=128",
					},
				},
			},
		}
	})
}
