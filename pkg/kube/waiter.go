package kube

import (
	"time"

	"github.com/salsadigitalauorg/rockpool/pkg/command"
	log "github.com/sirupsen/logrus"
)

type Waiter struct {
	Stage       string
	ClusterName string
	Namespace   string
	Resource    string
	Condition   string
	Retries     int
	Delay       int
	Info        string
}

func (w Waiter) GetStage() string {
	return w.Stage
}

func (w Waiter) Execute() bool {
	logger := log.WithFields(log.Fields{
		"stage":     w.Stage,
		"cluster":   w.ClusterName,
		"namespace": w.Namespace,
		"resource":  w.Resource,
		"condition": w.Condition,
	})
	if w.Info != "" {
		logger.Info(w.Info)
	}

	resourceNotFound := true
	var failedErr error
	retries := w.Retries
	for resourceNotFound && retries > 0 {
		failedErr = nil
		_, err := Cmd(w.ClusterName, w.Namespace, "wait",
			"--for=condition="+w.Condition, w.Resource).Output()
		if err != nil {
			failedErr = err
			retries--
			time.Sleep(time.Duration(w.Delay) * time.Second)
			continue
		}
		resourceNotFound = false
	}
	if failedErr != nil {
		logger.WithError(command.GetMsgFromCommandError(failedErr)).
			Fatal("error while waiting")
	}

	return true
}
