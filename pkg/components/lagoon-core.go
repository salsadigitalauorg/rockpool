package components

import (
	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/config"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
	"github.com/salsadigitalauorg/rockpool/pkg/lagoon"
)

func init() {
	Add("lagoon-core", func() Component {

		lagoonValues := config.C.ToMap()
		lagoonValues["LagoonVersion"] = lagoon.Version
		return Component{
			Name:     "lagoon-core",
			CompType: ComponentTypeLagoonCore,
			InstallActions: []action.Action{
				helm.Installer{
					Info: "installing lagoon core",
					AddRepo: helm.HelmRepo{
						Name: "lagoon",
						Url:  "https://uselagoon.github.io/lagoon-charts/",
					},
					Namespace:          "lagoon-core",
					ReleaseName:        "lagoon-core",
					Chart:              "lagoon/lagoon-core",
					Args:               []string{"--create-namespace", "--wait", "--timeout", "30m0s"},
					ValuesTemplate:     "lagoon-core-values.yml.tmpl",
					ValuesTemplateVars: lagoonValues,
				},
			},
		}
	})
}
