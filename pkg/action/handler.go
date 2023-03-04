package action

import log "github.com/sirupsen/logrus"

type Handler struct {
	Stage     string
	Info      string
	LogFields log.Fields
	Func      func(logger *log.Entry) bool
}

func (h Handler) GetStage() string {
	return h.Stage
}

func (h Handler) Execute() bool {
	if h.Stage != "" {
		h.LogFields["stage"] = h.Stage
	}
	logger := log.WithFields(h.LogFields)
	if h.Info != "" {
		logger.Info(h.Info)
	}
	return h.Func(logger)
}
