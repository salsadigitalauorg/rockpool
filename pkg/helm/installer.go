package helm

import (
	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/components/templates"
	log "github.com/sirupsen/logrus"
)

type HelmRepo struct {
	Name string
	Url  string
}

type Installer struct {
	Stage              string
	Info               string
	ClusterName        string
	AddRepo            HelmRepo
	Namespace          string
	ReleaseName        string
	Chart              string
	Args               []string
	ValuesTemplate     string
	ValuesTemplateVars interface{}
}

func (i Installer) GetStage() string {
	return i.Stage
}

func (i Installer) Execute() bool {
	logger := log.WithFields(log.Fields{
		"stage":     i.Stage,
		"cluster":   i.ClusterName,
		"namespace": i.Namespace,
		"release":   i.ReleaseName,
		"chart":     i.Chart,
	})
	if i.Info != "" {
		logger.Info(i.Info)
	}

	FetchInstalledReleases(i.ClusterName)
	if i.AddRepo.Url != "" {
		logger.WithField("addRepo", i.AddRepo)
		err := Exec(i.ClusterName, "", "repo", "add", i.AddRepo.Name,
			i.AddRepo.Url).Run()
		if err != nil {
			logger.WithError(command.GetMsgFromCommandError(err)).
				Fatal("error adding helm repository")
		}
	}

	args := i.Args
	if i.ValuesTemplate != "" {
		valuesFile, err := templates.Render(i.ValuesTemplate, i.ValuesTemplateVars, "")
		if err != nil {
			logger.WithError(err).Fatal("error rendering values template")
		}
		args = append(args, "-f", valuesFile)
	}

	err := InstallOrUpgrade(i.ClusterName, i.Namespace, i.ReleaseName, i.Chart, args)
	if err != nil {
		logger.WithError(err).Fatal("unable to install helm chart")
	}
	return true
}
