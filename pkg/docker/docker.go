package docker

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/salsadigitalauorg/rockpool/pkg/command"

	log "github.com/sirupsen/logrus"
)

var CurrentContext *Context

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
	contextLines := strings.Split(string(out), "\n")
	for _, l := range contextLines {
		if l == "" {
			continue
		}
		var context Context
		err = json.Unmarshal([]byte(l), &context)
		if err != nil {
			log.WithField("context-line", l).
				WithError(err).Fatal("unable to parse docker context line")
		}
		contexts = append(contexts, context)
	}
	log.WithField("contexts", contexts).Debug("parsed context list")

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

func Start(n string) ([]byte, error) {
	log.WithField("container", n).Debug("starting container")
	return command.ShellCommander("docker", "start", n).Output()
}

func Restart(n string) ([]byte, error) {
	log.WithField("container", n).Debug("restarting container")
	return command.ShellCommander("docker", "restart", n).Output()
}

func Inspect(n string) []Container {
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
	return containers
}

func Cp(src string, dest string) ([]byte, error) {
	log.WithFields(log.Fields{
		"src":  src,
		"dest": dest,
	}).Debug("copying files")
	return command.ShellCommander("docker", "cp", src, dest).Output()
}
