package helm

import (
	"encoding/json"
	"sync"

	"github.com/salsadigitalauorg/rockpool/internal"
	"github.com/salsadigitalauorg/rockpool/pkg/command"

	log "github.com/sirupsen/logrus"
)

// List of Helm releases per cluster.
var Releases sync.Map
var UpgradeComponents []string

func Exec(cn string, ns string, args ...string) command.IShellCommand {
	cmd := command.ShellCommander("helm", "--kubeconfig", internal.KubeconfigPath(cn))
	if ns != "" {
		cmd.AddArgs("--namespace", ns)
	}
	cmd.AddArgs(args...)
	return cmd
}

func List(cn string) {
	logger := log.WithField("clusterName", cn)
	out, err := Exec(cn, "", "list", "--all-namespaces", "--output", "json").Output()
	if err != nil {
		logger.WithFields(log.Fields{
			"out": string(out),
			"err": command.GetMsgFromCommandError(err),
		}).Fatal("unable to get list of helm releases")
	}
	releases := []HelmRelease{}
	err = json.Unmarshal(out, &releases)
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to parse helm releases")
	}
	Releases.Store(cn, releases)
}

func GetReleases(key string) []HelmRelease {
	logger := log.WithField("key", key)
	valueIfc, ok := Releases.Load(key)
	if !ok {
		logger.Panic("releases not found")
	}
	val, ok := valueIfc.([]HelmRelease)
	if !ok {
		logger.WithField("valueIfc", valueIfc).
			Panic("unable to convert binpath to string")
	}
	return val
}

func InstallOrUpgrade(cn string, ns string, releaseName string, chartName string, args []string) error {
	logger := log.WithFields(log.Fields{
		"clusterName": cn,
		"namespace":   ns,
		"releaseName": releaseName,
		"chartName":   chartName,
		"args":        args,
	})
	upgrade := false
	for _, u := range UpgradeComponents {
		if u == "all" || u == releaseName {
			upgrade = true
			break
		}
	}
	if !upgrade {
		for _, r := range GetReleases(cn) {
			if r.Name == releaseName {
				logger.Debug("helm release is already installed")
				return nil
			}
		}
	} else {
		logger.Info("upgrading")
	}

	// New install.
	if !upgrade {
		logger.Info("installing")
	}

	cmd := Exec(cn, ns, "upgrade", "--install", releaseName, chartName)
	cmd.AddArgs(args...)
	logger.WithField("command", cmd).Debug("running command for helm release")
	return cmd.RunProgressive()
}
