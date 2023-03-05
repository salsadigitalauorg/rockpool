package kube

import log "github.com/sirupsen/logrus"

type Applyer struct {
	Stage       string
	Info        string
	ClusterName string
	Namespace   string
	Template    string
	Urls        []string
	Force       bool
	Retries     int
	Delay       int
}

func (t Applyer) GetStage() string {
	return t.Stage
}

func (t Applyer) Execute() bool {
	logger := log.WithFields(log.Fields{
		"stage":     t.Stage,
		"cluster":   t.ClusterName,
		"namespace": t.Namespace,
		"template":  t.Template,
	})
	if t.Template != "" {
		log.WithField("template", t.Template)
	}
	if len(t.Urls) > 0 {
		log.WithField("urls", t.Urls)
	}
	if t.Info != "" {
		logger.Info(t.Info)
	}

	if t.Template != "" {
		ApplyTemplate(t.ClusterName, t.Namespace, t.Template, t.Force, t.Retries, t.Delay)
	}

	if len(t.Urls) > 0 {
		for _, u := range t.Urls {
			Apply(t.ClusterName, t.Namespace, u, t.Force)
		}
	}
	return true
}
