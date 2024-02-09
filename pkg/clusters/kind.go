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
}

var kindcp = KindClusterProvider{
	requiredBinaries: []action.BinaryExists{{Bin: "kind"}},
}

// Get the list of required binaries.
func (cp KindClusterProvider) GetRequiredBinaries() []action.BinaryExists {
	return cp.requiredBinaries
}

// Get the list of clusters.
func (cp KindClusterProvider) Clusters() []string {
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
		return []string{}
	}

	clusters := []string{}
	cluster_lines := strings.Split(string(out), "\n")
	for _, l := range cluster_lines {
		if l == "" {
			continue
		}
		clusters = append(clusters, l)
	}
	return clusters
}

// Get the list of nodes in a cluster.
func (cp KindClusterProvider) ClusterNodes(clusterName string) map[string]ClusterNode {
	log.Debug("fetching cluster nodes")
	out, err := command.
		ShellCommander("kind", "get", "nodes", "--name", clusterName).
		Output()
	if err != nil {
		log.WithField("name", clusterName).
			WithError(err).Fatal("error getting cluster nodes")
	}

	log.WithField("clusters-nodes-output", string(out)).
		Debug("got cluster nodes list")

	nodes := map[string]ClusterNode{}
	node_lines := strings.Split(string(out), "\n")
	for _, l := range node_lines {
		if l == "" {
			continue
		}
		container := docker.Inspect(l)
		nodes[l] = ClusterNode{Status: container.State.Status}
	}

	return nodes
}

// Exist checks if a cluster exist.
func (cp KindClusterProvider) Exist(clusterName string) bool {
	clusters := cp.Clusters()
	for _, c := range clusters {
		if c == clusterName {
			return true
		}
	}
	return false
}

// Get the status of the cluster.
func (cp KindClusterProvider) Status(clusterName string) {
	if !cp.Exist(clusterName) {
		log.WithField("name", clusterName).
			Fatal("cluster does not exist")
	}
	log.Println("cluster:", clusterName)

	nodes := cp.ClusterNodes(clusterName)
	log.Println("cluster nodes: ")
	for nodeName, node := range nodes {
		log.Printf("  %s (%s)", nodeName, node.Status)
	}
}

// Create a cluster.
func (cp KindClusterProvider) Create(clusterName string) {
	if cp.Exist(clusterName) {
		log.WithField("cluster", clusterName).
			Info("cluster already exists")
		cp.Start(clusterName)
		return
	}

	log.Debug("creating cluster")
	err := command.
		ShellCommander("kind", "create", "cluster", "-n", clusterName).
		RunProgressive()
	if err != nil {
		log.WithField("cluster", clusterName).WithError(err).Fatal("error creating cluster")
	}
}

// Start a cluster.
func (cp KindClusterProvider) Start(clusterName string) {
	if !cp.Exist(clusterName) {
		log.WithField("cluster", clusterName).Fatal("cluster does not exist")
	}

	nodes := cp.ClusterNodes(clusterName)
	if len(nodes) == 0 {
		log.WithField("cluster", clusterName).
			Fatal("no node to start - cluster not created?")
	}

	for nodeName, node := range nodes {
		if node.Status == "running" {
			log.WithField("node", nodeName).Debug("node already running")
			continue
		}
		log.WithField("node", nodeName).Info("starting node")
		err := docker.Start(nodeName).RunProgressive()
		if err != nil {
			log.WithField("node", nodeName).
				WithError(err).Fatal("error starting node")
		}
	}
}

// Stop a cluster.
func (cp KindClusterProvider) Stop(clusterName string) {
	if !cp.Exist(clusterName) {
		log.WithField("cluster", clusterName).Debug("cluster does not exist")
		return
	}

	nodes := cp.ClusterNodes(clusterName)
	if len(nodes) == 0 {
		log.WithField("cluster", clusterName).Debug("no node to stop")
		return
	}

	for nodeName := range nodes {
		log.WithField("node", nodeName).Debug("stopping node")
		err := docker.Stop(nodeName).RunProgressive()
		if err != nil {
			log.WithField("node", nodeName).
				WithError(err).Fatal("error stopping node")
		}
	}
}

// Delete a cluster.
func (cp KindClusterProvider) Delete(clusterName string) {
	if !cp.Exist(clusterName) {
		log.WithField("cluster", clusterName).Debug("cluster does not exist")
		return
	}

	nodes := cp.ClusterNodes(clusterName)
	if len(nodes) == 0 {
		log.WithField("cluster", clusterName).Debug("no node to delete")
		return
	}

	for nodeName := range nodes {
		log.WithField("node", nodeName).Debug("deleting node")
		err := docker.Remove(nodeName).RunProgressive()
		if err != nil {
			log.WithField("node", nodeName).
				WithError(err).Fatal("error deleting cluster nodes")
		}
	}
}
