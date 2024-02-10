package components

import (
	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/config"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
)

func init() {
	Add("harbor", func() Component {
		return Component{
			Name:     "harbor",
			CompType: ComponentTypeLagoonReq,
			InstallActions: []action.Action{
				helm.Installer{
					Stage: "controller-setup",
					Info:  "installing harbor",
					AddRepo: helm.HelmRepo{
						Name: "harbor",
						Url:  "https://helm.goharbor.io",
					},
					Namespace:          "harbor",
					ReleaseName:        "harbor",
					Chart:              "harbor/harbor",
					Args:               []string{"--create-namespace", "--wait"},
					ValuesTemplate:     "harbor-values.yml.tmpl",
					ValuesTemplateVars: config.C.ToMap(),
				},
			},
		}
	})
}
