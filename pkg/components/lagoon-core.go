package components

import (
	log "github.com/sirupsen/logrus"

	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/config"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
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
				action.Handler{
					Info: "ensuring db tables have been created",
					Func: func(logger *log.Entry) bool {
						cn := logger.Data["cluster"].(string)

						logger.Debug("checking if tables exist")
						out, err := kube.Cmd(cn, "lagoon-core", "exec",
							"sts/lagoon-core-api-db", "--", "bash", "-c",
							"mysql -u$MARIADB_USER -p$MARIADB_PASSWORD $MARIADB_DATABASE -e 'SHOW TABLES;'",
						).Output()
						if err != nil {
							logger.WithError(command.GetMsgFromCommandError(err)).
								Fatal("error getting tables")
						}
						if string(out) != "" {
							return true
						}

						logger.Debug("running the db init script")
						err = kube.Cmd(cn, "lagoon-core", "exec", "sts/lagoon-core-api-db",
							"--", "/legacy_rerun_initdb.sh").Run()
						if err != nil {
							logger.WithError(command.GetMsgFromCommandError(err)).
								Fatal("error running db init")
						}

						return true
					},
				},
			},
		}
	})
}
