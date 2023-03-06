package docker

import (
	"encoding/json"

	"github.com/salsadigitalauorg/rockpool/pkg/command"

	log "github.com/sirupsen/logrus"
)

func Exec(n string, cmdStr string) command.IShellCommand {
	return command.ShellCommander("docker", "exec", n, "ash", "-c", cmdStr)
}

func Stop(n string) ([]byte, error) {
	log.WithField("container", n).Debug("stopping container")
	return command.ShellCommander("docker", "stop", n).Output()
}

func Start(n string) ([]byte, error) {
	log.WithField("container", n).Debug("starting container")
	return command.ShellCommander("docker", "start", n).Output()
}

func Restart(n string) ([]byte, error) {
	log.WithField("container", n).Debug("restarting container")
	return command.ShellCommander("docker", "restart", n).Output()
}

func Inspect(n string) []DockerContainer {
	log.WithField("container", n).Debug("inspecting container")
	cmd := command.ShellCommander("docker", "inspect", n)
	out, err := cmd.Output()
	if err != nil {
		log.WithFields(log.Fields{
			"container": n,
			"out":       string(out),
		}).WithError(command.GetMsgFromCommandError(err)).
			Fatal("unable to get list of docker containers")
	}
	containers := []DockerContainer{}
	err = json.Unmarshal(out, &containers)
	if err != nil {
		log.WithFields(log.Fields{
			"container": n,
			"err":       err,
		}).Fatal("unable to parse docker containers")
	}
	return containers
}

func Cp(src string, dest string) ([]byte, error) {
	log.WithFields(log.Fields{
		"src":  src,
		"dest": dest,
	}).Debug("copying files")
	return command.ShellCommander("docker", "cp", src, dest).Output()
}
