package kube

import log "github.com/sirupsen/logrus"

type Templater struct {
	Stage       string
	ClusterName string
	Namespace   string
	Template    string
	Force       bool
	Retries     int
	Delay       int
	Info        string
}

func (t Templater) GetStage() string {
	return t.Stage
}

func (t Templater) Execute() bool {
	logger := log.WithFields(log.Fields{
		"stage":     t.Stage,
		"cluster":   t.ClusterName,
		"namespace": t.Namespace,
		"template":  t.Template,
	})
	if t.Info != "" {
		logger.Info(t.Info)
	}
	ApplyTemplate(t.ClusterName, t.Namespace, t.Template, t.Force, t.Retries, t.Delay)
	return true
}
