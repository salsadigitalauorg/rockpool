package action

import log "github.com/sirupsen/logrus"

type Chain struct {
	Actions          []Action
	ErrorMsg         string
	FailOnFirstError *bool
}

type Action interface {
	GetStage() string
	Execute() bool
}

func (c Chain) Add(a Action) *Chain {
	c.Actions = append(c.Actions, a)
	return &c
}

func (c Chain) Run() {
	if c.FailOnFirstError == nil {
		c.FailOnFirstError = &[]bool{true}[0]
	}
	failureEncountered := false
	for _, a := range c.Actions {
		success := a.Execute()
		if !success && *c.FailOnFirstError {
			log.Fatal(c.ErrorMsg)
		} else if !success {
			failureEncountered = true
		}
	}
	if failureEncountered {
		log.Fatal(c.ErrorMsg)
	}
}
