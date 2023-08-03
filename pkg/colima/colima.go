package colima

import (
	"encoding/json"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/command"
)

var RequiredBinaries = []action.BinaryExists{{Bin: "colima"}}

// Get the list of clusters.
func List() []Cluster {
	log.Debug("fetching cluster list")
	out, err := command.
		ShellCommander("colima", "list", "--json").
		Output()
	if err != nil {
		log.WithError(command.GetMsgFromCommandError(err)).
			Fatal("unable to get cluster list")
	}

	log.WithField("clusters-output", string(out)).Debug("got cluster list")

	// Parse the JSON output and return a ClusterList.
	var clusters []Cluster
	clusterLines := strings.Split(string(out), "\n")
	for _, l := range clusterLines {
		if l == "" {
			continue
		}
		var cluster Cluster
		err = json.Unmarshal([]byte(l), &cluster)
		if err != nil {
			log.WithField("cluster-line", l).
				WithError(err).Fatal("unable to parse cluster line")
		}
		clusters = append(clusters, cluster)
	}

	log.WithField("clusters", clusters).Debug("parsed cluster list")

	return clusters
}

// Get the status of the cluster.
func Status(name string) {
	err := command.
		ShellCommander("colima", "status", "--profile", name, "--extended").
		RunProgressive()
	if err != nil {
		log.WithField("name", name).
			WithError(err).Fatal("error getting cluster status")
	}
}

// Create a cluster.
func Create(name string) {
	log.WithField("name", name).Debug("creating cluster")
	err := command.
		ShellCommander("colima", "start", "--profile", name, "--kubernetes").
		RunProgressive()
	if err != nil {
		log.WithField("name", name).
			WithError(err).Fatal("error creating cluster")
	}
}

// Stop a cluster.
func Stop(name string) {
	log.WithField("name", name).Debug("stopping cluster")
	err := command.
		ShellCommander("colima", "stop", "--profile", name).
		RunProgressive()
	if err != nil {
		log.WithField("name", name).
			WithError(err).Fatal("error stopping cluster")
	}
}
