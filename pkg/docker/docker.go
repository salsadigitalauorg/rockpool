package docker

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/salsadigitalauorg/rockpool/pkg/command"

	log "github.com/sirupsen/logrus"
)

var CurrentContext *Context

func GetProvider() Provider {
	out, err := command.ShellCommander("docker", "version", "--format", "json").Output()
	if err != nil {
		log.WithError(command.GetMsgFromCommandError(err)).
			Fatal("unable to get docker version")
	}

	log.WithField("docker-version-output", string(out)).
		Debug("got docker version")
	var version DockerVersion
	err = json.Unmarshal(out, &version)
	if err != nil {
		log.WithError(err).Fatal("unable to parse docker version")
	}

	log.WithField("version", version).Debug("parsed docker version")
	if version.Client.Context == "desktop-linux" &&
		strings.Contains(version.Server.Platform.Name, "Docker Desktop") {
		return ProviderDockerDesktop
	} else if version.Client.Context == "colima" &&
		version.Server.Platform.Name == "Docker Engine - Community" {
		return ProviderColima
	} else if version.Client.Context == "default" &&
		strings.Contains(version.Client.Version, "-rd") {
		return ProviderRancherDesktop
	}

	log.WithField("version", version).
		Fatal("unable to determine docker provider")
	return ""
}

func GetCurrentContext() *Context {
	if CurrentContext != nil {
		return CurrentContext
	}

	out, err := command.ShellCommander("docker", "context", "ls", "--format", "json").Output()
	if err != nil {
		log.WithError(command.GetMsgFromCommandError(err)).
			Fatal("unable to get docker context list")
	}

	log.WithField("contexts-output", string(out)).Debug("got context list")

	var contexts []Context
	outStr := strings.Trim(string(out), "\n")
	err = json.Unmarshal([]byte(outStr), &contexts)
	if err != nil {
		log.WithField("contexts-out", outStr).
			WithError(err).Fatal("unable to parse docker contexts")
	}
	log.WithField("contexts", contexts).Debug("parsed contexts")

	for _, c := range contexts {
		if !c.Current {
			continue
		}
		log.WithField("context", c).Debug("current context")
		CurrentContext = &c
		return CurrentContext
	}
	return nil
}

func ColimaGetProfiles() []ColimaProfile {
	out, err := command.ShellCommander("colima", "ls", "--json").Output()
	if err != nil {
		log.WithError(command.GetMsgFromCommandError(err)).
			Fatal("unable to get colima profiles")
	}

	var profiles []ColimaProfile
	colimaStrs := strings.Split(string(out), "\n")
	for _, c := range colimaStrs {
		if strings.Trim(c, " \n") == "" {
			continue
		}
		var profile ColimaProfile
		err = json.Unmarshal([]byte(c), &profile)
		if err != nil {
			log.WithField("line", c).WithError(err).
				Fatal("unable to parse colima profiles")
		}
		profiles = append(profiles, profile)
	}
	log.WithField("profiles", profiles).Debug()
	return profiles
}

func GetVmIp() string {
	// Check if colima is being used.
	currentContext := GetCurrentContext()

	var colimaProfileName string
	if currentContext.Description == "colima" {
		colimaProfileName = "default"
	} else {
		r, _ := regexp.Compile(`colima-?([a-zA-z0-9]+)[ \\*]*`)
		match := r.FindStringSubmatch(currentContext.Name)
		if match != nil {
			colimaProfileName = match[1]
		}
	}
	log.WithField("colimaProfileName", colimaProfileName).Debug()

	if colimaProfileName != "" {
		profiles := ColimaGetProfiles()
		for _, p := range profiles {
			if p.Name != colimaProfileName {
				continue
			}
			return p.Address
		}
	}
	return "127.0.0.1"
}

func Exec(n string, cmdStr string) command.IShellCommand {
	return command.ShellCommander("docker", "exec", n, "ash", "-c", cmdStr)
}

func Stop(n string) command.IShellCommand {
	log.WithField("container", n).Debug("stopping container")
	return command.ShellCommander("docker", "stop", n)
}

func Start(n string) command.IShellCommand {
	log.WithField("container", n).Debug("starting container")
	return command.ShellCommander("docker", "start", n)
}

func Restart(n string) ([]byte, error) {
	log.WithField("container", n).Debug("restarting container")
	return command.ShellCommander("docker", "restart", n).Output()
}

func Remove(n string) command.IShellCommand {
	log.WithField("container", n).Debug("deleting container")
	return command.ShellCommander("docker", "rm", n)
}

func Ps(label string) ([]PsContainer, error) {
	log.WithField("label", label).Debug("getting containers")
	cmd := command.ShellCommander("docker", "ps", "--format", "json")
	if label != "" {
		cmd.AddArgs("--filter", "label="+label)
	}
	out, err := cmd.Output()
	if err != nil {
		log.WithFields(log.Fields{
			"label": label,
			"out":   string(out),
		}).WithError(command.GetMsgFromCommandError(err)).
			Fatal("unable to get list of docker containers")
	}
	containers := []PsContainer{}
	err = json.Unmarshal(out, &containers)
	if err != nil {
		log.WithFields(log.Fields{
			"label": label,
			"err":   err,
		}).Fatal("unable to parse docker containers")
	}
	return containers, err
}

func Inspect(n string) Container {
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

	containers := []Container{}
	err = json.Unmarshal(out, &containers)
	if err != nil {
		log.WithFields(log.Fields{
			"container": n,
			"err":       err,
		}).Fatal("unable to parse docker containers")
	}

	log.WithField("docker-inspect-parsed", containers).
		Debug("got inspected container")
	return containers[0]
}

func Cp(src string, dest string) ([]byte, error) {
	log.WithFields(log.Fields{
		"src":  src,
		"dest": dest,
	}).Debug("copying files")
	return command.ShellCommander("docker", "cp", src, dest).Output()
}
