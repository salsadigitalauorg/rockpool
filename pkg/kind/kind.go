package kind

import (
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/docker"
)

var RequiredBinaries = []action.BinaryExists{{Bin: "kind"}}

var ClusterNodes map[string][]string

// Get the list of clusters and nodes.
func List() map[string][]string {
	if ClusterNodes != nil {
		return ClusterNodes
	}
	ClusterNodes = make(map[string][]string)

	log.Debug("fetching cluster list")
	out, err := command.
		ShellCommander("kind", "get", "clusters").
		Output()
	if err != nil {
		log.WithError(command.GetMsgFromCommandError(err)).
			Fatal("unable to get cluster list")
	}

	log.WithField("clusters-output", string(out)).Debug("got cluster list")
	if string(out) == "No kind clusters found." {
		return ClusterNodes
	}

	cluster_lines := strings.Split(string(out), "\n")
	for _, l := range cluster_lines {
		if l == "" {
			continue
		}
		ClusterNodes[l] = Nodes(l)
	}

	return ClusterNodes
}

// Get the list of nodes in a cluster.
func Nodes(name string) []string {
	log.Debug("fetching cluster nodes")
	out, err := command.
		ShellCommander("kind", "get", "nodes", "--name", name).
		Output()
	if err != nil {
		log.WithField("name", name).
			WithError(err).Fatal("error getting cluster nodes")
	}

	log.WithField("clusters-nodes-output", string(out)).
		Debug("got cluster nodes list")

	ClusterNodes[name] = []string{}
	node_lines := strings.Split(string(out), "\n")
	for _, l := range node_lines {
		if l == "" {
			continue
		}
		ClusterNodes[name] = append(ClusterNodes[name], l)
	}

	return ClusterNodes[name]
}

// Get the status of the cluster.
func Status(name string) {
	List()
	if _, ok := ClusterNodes[name]; !ok {
		log.WithField("name", name).
			Fatal("cluster does not exist")
	}
	log.Print("cluster nodes: ", ClusterNodes[name])
}

// Create a cluster.
func Create(name string) {
	List()
	if _, ok := ClusterNodes[name]; ok {
		log.WithField("name", name).
			Info("cluster already exists")
		return
	}
	log.Debug("creating cluster")
	err := command.
		ShellCommander("kind", "create", "cluster", "-n", name).
		RunProgressive()
	if err != nil {
		log.WithError(err).Fatal("error creating cluster")
	}
}

// Stop a cluster.
func Stop(name string) {
	List()
	log.Debug("stopping cluster nodes")
	for _, node := range ClusterNodes[name] {
		err := docker.Stop(node).RunProgressive()
		if err != nil {
			log.WithError(err).Fatal("error stopping cluster nodes")
		}
	}
}

// Delete a cluster.
func Delete(name string) {
	log.Debug("deleting cluster nodes")
	for _, node := range ClusterNodes[name] {
		err := docker.Remove(node).RunProgressive()
		if err != nil {
			log.WithError(err).Fatal("error deleting cluster nodes")
		}
	}
}
