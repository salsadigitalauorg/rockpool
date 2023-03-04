package helm

import log "github.com/sirupsen/logrus"

type Installer struct {
	Stage       string
	ClusterName string
	Namespace   string
	ReleaseName string
	Chart       string
	Args        []string
	Info        string
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
	err := InstallOrUpgrade(i.ClusterName, i.Namespace, i.ReleaseName,
		i.Chart, i.Args)
	if err != nil {
		logger.WithError(err).Fatal("unable to install helm chart")
	}
	return true
}
