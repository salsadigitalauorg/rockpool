package components

import (
	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/config"
	"github.com/salsadigitalauorg/rockpool/pkg/gitea"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"

	log "github.com/sirupsen/logrus"
)

func init() {
	Add("gitea", func() Component {
		return Component{
			Name:     "gitea",
			CompType: ComponentTypeLocalReq,
			InstallActions: []action.Action{
				helm.Installer{
					Info: "installing gitea",
					AddRepo: helm.HelmRepo{
						Name: "gitea-charts",
						Url:  "https://dl.gitea.io/charts/",
					},
					Namespace:          "gitea",
					ReleaseName:        "gitea",
					Chart:              "gitea-charts/gitea",
					Args:               []string{"--create-namespace", "--wait"},
					ValuesTemplate:     "gitea-values.yml.tmpl",
					ValuesTemplateVars: config.C.ToMap(),
				},
				action.Handler{
					Func: func(logger *log.Entry) bool {
						// Create test repo.
						gitea.CreateRepo()
						return true
					},
				},
			},
		}
	})
}
