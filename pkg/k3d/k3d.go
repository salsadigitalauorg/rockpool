package k3d

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/docker"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	"github.com/salsadigitalauorg/rockpool/pkg/platform/templates"

	log "github.com/sirupsen/logrus"
)

var registryName = "rockpool-registry"
var registryNameFull = "k3d-" + registryName

var registries []Registry
var Reg Registry

var Clusters ClusterList

func RegistryList() {
	log.Info("fetching registry list")
	res, err := command.
		ShellCommander("k3d", "registry", "list", "-o", "json").
		Output()
	if err != nil {
		log.WithField("error", command.GetMsgFromCommandError(err)).
			Fatal("[rockpool] unable to get registry list")
	}

	err = json.Unmarshal(res, &registries)
	if err != nil {
		log.WithField("error", command.GetMsgFromCommandError(err)).
			Fatal("[rockpool] unable to parse registry list")
	}
}

func RegistryGet() {
	RegistryList()
	for _, reg := range registries {
		if reg.Name == registryNameFull {
			Reg = reg
			break
		}
	}
}

func RegistryCreate() {
	RegistryGet()
	if Reg.Name == registryNameFull {
		log.WithField("registry", registryNameFull).
			Info("[rockpool] registry already exists")
		return
	}

	log.WithField("registry", registryNameFull).
		Info("[rockpool] creating registry")
	_, err := command.ShellCommander(
		"k3d", "registry", "create", registryName, "--port", "5111").Output()
	if err != nil {
		log.WithFields(log.Fields{
			"registry": registryNameFull,
			"err":      command.GetMsgFromCommandError(err),
		}).Fatal("[rockpool] unable to create registry")
	}

	// Configure registry to enable proxy.
	regCfgFile := "/etc/docker/registry/config.yml"
	proxyLine := "proxy:\n  remoteurl: https://registry-1.docker.io"
	proxyLineCmdStr := fmt.Sprintf("echo '%s' >> "+regCfgFile, proxyLine)

	var registryConfig []byte
	done := false
	retries := 12
	for !done && retries > 0 {
		registryConfig, err = docker.Exec(registryNameFull, "cat "+regCfgFile)
		if err != nil {
			log.WithFields(log.Fields{
				"registry": registryNameFull,
				"err":      command.GetMsgFromCommandError(err),
			}).Warn("[rockpool] unable to find registry container")
			time.Sleep(5 * time.Second)
			retries--
			continue
		}
		done = true
	}
	if err != nil {
		log.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("[rockpool] unable to find registry container")
	}

	if !strings.Contains(string(registryConfig), proxyLine) {
		log.WithFields(log.Fields{
			"registry":  registryNameFull,
			"proxyLine": proxyLine,
		}).Info("[rockpool] adding registry proxy config")
		_, err := docker.Exec(registryNameFull, proxyLineCmdStr)
		if err != nil {
			log.WithFields(log.Fields{
				"registry":  registryNameFull,
				"proxyLine": proxyLine,
				"err":       command.GetMsgFromCommandError(err),
			}).Fatal("[rockpool] error adding registry proxy config")
		}
		docker.Restart(registryNameFull)
	}
}

func RegistryRenderConfig() {
	_, err := templates.Render("registries.yaml", nil, "")
	if err != nil {
		panic(err)
	}
}

func RegistryStop() {
	RegistryGet()
	if Reg.Name != registryNameFull {
		return
	}
	log.WithField("registry", registryNameFull).Info("[rockpool] stopping registry")
	_, err := docker.Stop(Reg.Name)
	if err != nil {
		log.WithFields(log.Fields{
			"registry": registryNameFull,
			"err":      command.GetMsgFromCommandError(err),
		}).Fatal("[rockpool] error stopping registry")
	}
	log.WithField("registry", registryNameFull).Info("[rockpool] stopped registry")
}

func RegistryStart() {
	log.WithField("registry", registryNameFull).
		Info("[rockpool] starting registry")
	_, err := docker.Start(registryNameFull)
	if err != nil {
		log.WithFields(log.Fields{
			"registry": registryNameFull,
			"err":      command.GetMsgFromCommandError(err),
		}).Fatal("[rockpool] error starting registry")
	}
	log.WithField("registry", registryNameFull).Info("[rockpool] started registry")
}

func RegistryDelete() {
	RegistryGet()
	if Reg.Name != registryNameFull {
		return
	}
	log.WithField("registry", Reg.Name).Info("[rockpool] deleting registry")
	_, err := command.ShellCommander("k3d", "registry", "delete", Reg.Name).Output()
	if err != nil {
		log.WithFields(log.Fields{
			"registry": Reg.Name,
			"err":      command.GetMsgFromCommandError(err),
		}).Fatal("[rockpool] unable to delete registry")
	}
	log.WithField("registry", Reg.Name).Info("[rockpool] deleted registry")
}

func ClusterFetchAll() ClusterList {
	var cl ClusterList
	res, err := command.ShellCommander("k3d", "cluster", "list", "-o", "json").Output()
	log.Debug("[k3d] cluster list: ", string(res))
	if err != nil {
		log.Fatal("[rockpool] unable to get cluster list: %s\n", command.GetMsgFromCommandError(err))
	}

	err = json.Unmarshal(res, &cl)
	if err != nil {
		log.Fatal("[rockpool] unable to parse cluster list: %s\n", err)
	}
	return cl
}

func ClusterExists(clusterName string) (bool, Cluster) {
	for _, c := range Clusters {
		if c.Name == clusterName {
			return true, c
		}
	}
	return false, Cluster{}
}

func ClusterFetch() {
	log.Info("fetching clusters")
	for _, c := range ClusterFetchAll() {
		if !strings.HasPrefix(c.Name, platform.Name) {
			continue
		}
		// Skip if already present.
		if exists, _ := ClusterExists(c.Name); exists {
			continue
		}
		Clusters = append(Clusters, c)
	}
}

func ClusterIsRunning(clusterName string) bool {
	ClusterFetch()
	log.Info("checking if cluster is running")
	for _, c := range Clusters {
		if c.Name != clusterName {
			continue
		}
		return c.AgentsCount == c.AgentsRunning && c.ServersCount == c.ServersRunning
	}
	return false
}

func ClusterCreate(cn string, isController bool) {
	logger := log.WithFields(log.Fields{
		"clusterName":  cn,
		"isController": isController,
	})
	ClusterFetch()
	if exists, _ := ClusterExists(cn); exists && ClusterIsRunning(cn) {
		logger.Info("cluster already exists and is running")
		return
	} else if exists {
		logger.Info("cluster already exists, but is stopped; starting now")
		ClusterStart(cn)
		return
	}

	k3sArgs := []string{"--k3s-arg", "--disable=traefik@server:0"}
	cmdArgs := []string{
		"cluster", "create", "--kubeconfig-update-default=false",
		"--image=ghcr.io/salsadigitalauorg/rockpool/k3s:latest",
		"--agents", "1", "--network", "k3d-rockpool",
		"--registry-use", registryName + ":5000",
		"--registry-config", fmt.Sprintf("%s/registries.yaml", templates.RenderedPath(false)),
	}

	if isController {
		cmdArgs = append(cmdArgs,
			"--port", "80:80@loadbalancer",
			"--port", "443:443@loadbalancer",
			"--port", "2022:22@loadbalancer",
			// Required for cross-cluster amqp.
			"--port", "5672:5672@loadbalancer",
			"--port", "6153:6153/udp@loadbalancer",
			"--port", "6153:6153/tcp@loadbalancer",
		)
	} else { // Target cluster exposed ports.
		cmdArgs = append(cmdArgs,
			// Expose arbitrary ports for ingress-nginx.
			"--port", "80@loadbalancer",
			"--port", "443@loadbalancer",
		)
	}

	cmdArgs = append(cmdArgs, k3sArgs...)
	cmdArgs = append(cmdArgs, cn)
	cmd := command.ShellCommander("k3d", cmdArgs...)

	logger.WithField("command", cmd).Info("creating cluster")
	err := cmd.RunProgressive()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to create cluster")
	}
	logger.Info("created cluster")
	ClusterFetch()
}

func ClusterStart(cn string) {
	logger := log.WithField("clusterName", cn)
	if exists, _ := ClusterExists(cn); !exists {
		logger.Info("cluster does not exist")
		return
	}
	logger.Info("starting cluster")
	err := command.ShellCommander("k3d", "cluster", "start", cn).RunProgressive()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to start cluster")
	}
	ClusterFetch()
	logger.Info("started cluster")
}

func ClusterStop(cn string) {
	logger := log.WithField("clusterName", cn)
	if exists, _ := ClusterExists(cn); !exists {
		logger.Info("cluster does not exist")
		return
	}
	logger.Info("stopping cluster")
	err := command.ShellCommander("k3d", "cluster", "stop", cn).RunProgressive()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to stop cluster")
	}
	ClusterFetch()
	logger.Info("stopped cluster")
}

func ClusterRestart(cn string) {
	ClusterStop(cn)
	ClusterStart(cn)
}

func ClusterDelete(cn string) {
	logger := log.WithField("clusterName", cn)
	defer platform.WgDone()
	if exists, _ := ClusterExists(cn); !exists {
		return
	}
	ClusterStop(cn)
	logger.Info("deleting cluster")
	_, err := command.ShellCommander("k3d", "cluster", "delete", cn).Output()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to delete cluster")
	}
	ClusterFetch()
	logger.Info("deleted cluster")
}

func WriteKubeConfig(cn string) {
	logger := log.WithField("clusterName", cn)
	logger.Info("writing kubeconfig")
	_, err := command.ShellCommander("k3d", "kubeconfig", "write", cn).Output()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Panic("unable to write kubeconfig:")
	}
}

func ControllerIP() string {
	for _, c := range Clusters {
		if c.Name != platform.ControllerClusterName() {
			continue
		}

		for _, n := range c.Nodes {
			if n.Role == "loadbalancer" {
				return n.IP.IP
			}
		}
	}
	log.Fatal("[rockpool] unable to get controller ip")
	return ""
}

func TargetIP(cn string) string {
	for _, c := range Clusters {
		if c.Name != cn {
			continue
		}

		for _, n := range c.Nodes {
			if n.Role == "loadbalancer" {
				return n.IP.IP
			}
		}
	}
	log.Fatal("[rockpool] unable to get target ip")
	return ""
}
