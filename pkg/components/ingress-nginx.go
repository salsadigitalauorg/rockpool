package components

import (
	"fmt"

	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
)

var IngressNginxDefaultVersion = "4.5.2"

func init() {
	Add("ingress-nginx", Component{
		Name:     "ingress-nginx",
		CompType: ComponentTypeLagoonReq,
		InstallActions: []action.Action{
			helm.Installer{
				Info:        "installing ingress-nginx",
				Namespace:   "ingress-nginx",
				ReleaseName: "ingress-nginx",
				Chart: fmt.Sprintf(
					"https://github.com/kubernetes/ingress-nginx/releases/download/"+
						"helm-chart-%s/ingress-nginx-%s.tgz",
					IngressNginxDefaultVersion,
					IngressNginxDefaultVersion),
				Args: []string{
					"--create-namespace", "--wait",
					"--set", "controller.config.ssl-redirect=false",
					"--set", "controller.config.proxy-body-size=8m",
					"--set", "controller.ingressClassResource.default=true",
					"--set", "controller.watchIngressWithoutClass=true",
					"--set", "server-name-hash-bucket-size=128",
				},
			},
		},
	})
}
