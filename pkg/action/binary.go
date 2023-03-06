package action

import (
	"os/exec"

	"github.com/salsadigitalauorg/rockpool/pkg/command"

	log "github.com/sirupsen/logrus"
)

type BinaryExists struct {
	Stage       string
	Bin         string
	VersionArgs []string
}

func (b BinaryExists) GetStage() string {
	return b.Stage
}

func (b BinaryExists) Execute() bool {
	if b.Stage == "" {
		b.Stage = "init"
	}

	logger := log.WithFields(log.Fields{
		"stage": b.Stage,
		"bin":   b.Bin,
	})

	absPath, err := exec.LookPath(b.Bin)
	if err != nil {
		logger.WithError(err).
			Error("could not find binary; please ensure it is installed " +
				"and can be found in the $PATH")
		return false
	}

	versionCmd := command.ShellCommander(absPath, "version")
	if len(b.VersionArgs) > 0 {
		versionCmd.AddArgs(b.VersionArgs...)
	}
	out, err := versionCmd.Output()
	if err != nil {
		logger.WithError(command.GetMsgFromCommandError(err)).
			Error("error getting version")
		return false
	}

	logger.WithFields(log.Fields{
		"binary": b.Bin,
		"result": string(out),
	}).Debug("fetched version")
	return true
}
