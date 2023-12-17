package clusters

import (
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/docker"
)

type KindClusterProvider struct {
	requiredBinaries []action.BinaryExists
	clusterNodes     map[string][]string
}

var kindcp = KindClusterProvider{
	requiredBinaries: []action.BinaryExists{{Bin: "kind"}},
}

// Get the list of required binaries.
func (cp *KindClusterProvider) GetRequiredBinaries() []action.BinaryExists {
	return cp.requiredBinaries
}

// Get the list of clusters and nodes.
func (cp *KindClusterProvider) List() map[string][]string {
	if cp.clusterNodes != nil {
		return cp.clusterNodes
	}
	cp.clusterNodes = make(map[string][]string)

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
		return cp.clusterNodes
	}

	cluster_lines := strings.Split(string(out), "\n")
	for _, l := range cluster_lines {
		if l == "" {
			continue
		}
		cp.clusterNodes[l] = cp.Nodes(l)
	}

	return cp.clusterNodes
}

// Get the list of nodes in a cluster.
func (cp *KindClusterProvider) Nodes(name string) []string {
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

	cp.clusterNodes[name] = []string{}
	node_lines := strings.Split(string(out), "\n")
	for _, l := range node_lines {
		if l == "" {
			continue
		}
		cp.clusterNodes[name] = append(cp.clusterNodes[name], l)
	}

	return cp.clusterNodes[name]
}

// Exist checks if the platform clusters exist.
func (cp *KindClusterProvider) Exist(name string) bool {
	cp.List()
	_, exists := cp.clusterNodes[name]
	return exists
}

// Get the status of the cluster.
func (cp *KindClusterProvider) Status(name string) {
	cp.List()
	if _, ok := cp.clusterNodes[name]; !ok {
		log.WithField("name", name).
			Fatal("cluster does not exist")
	}
	log.Print("cluster nodes: ", cp.clusterNodes[name])
}

// Create a cluster.
func (cp *KindClusterProvider) Create(name string) {
	cp.List()
	if _, ok := cp.clusterNodes[name]; ok {
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
func (cp *KindClusterProvider) Stop(name string) {
	cp.List()
	log.Debug("stopping cluster nodes")
	for _, node := range cp.clusterNodes[name] {
		err := docker.Stop(node).RunProgressive()
		if err != nil {
			log.WithError(err).Fatal("error stopping cluster nodes")
		}
	}
}

// Delete a cluster.
func (cp *KindClusterProvider) Delete(name string) {
	log.Debug("deleting cluster nodes")
	for _, node := range cp.clusterNodes[name] {
		err := docker.Remove(node).RunProgressive()
		if err != nil {
			log.WithError(err).Fatal("error deleting cluster nodes")
		}
	}
}
