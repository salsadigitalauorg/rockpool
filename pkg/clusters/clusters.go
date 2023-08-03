package clusters

import (
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/colima"
	"github.com/salsadigitalauorg/rockpool/pkg/config"
	"github.com/salsadigitalauorg/rockpool/pkg/docker"
	"github.com/salsadigitalauorg/rockpool/pkg/kind"
)

var requiredBinaries = []action.BinaryExists{
	{Bin: "docker", VersionArgs: []string{"--format", "json"}},
	{Bin: "kubectl", VersionArgs: []string{"--client", "--short"}},
	{Bin: "helm"},
}

func VerifyRequirements() error {
	currentDockerContext := docker.GetCurrentContext()

	switch config.C.Clusters.Provider {
	case config.ClusterProviderKind:
		if strings.Contains(currentDockerContext.Name, "colima-") {
			log.Warn("please note that there is currently an issue preventing " +
				"kind from working on Colima for kind >=0.20.0. See this issue for more " +
				"information: https://github.com/kubernetes-sigs/kind/issues/3277")
		}
		requiredBinaries = append(requiredBinaries, kind.RequiredBinaries...)
	case config.ClusterProviderColima:
		requiredBinaries = append(requiredBinaries, colima.RequiredBinaries...)
	}

	log.Debug("checking if binaries exist")
	chain := &action.Chain{
		FailOnFirstError: &[]bool{false}[0],
		ErrorMsg:         "some requirements were not met; please review above",
	}
	for _, binary := range requiredBinaries {
		chain.Add(binary)
	}
	chain.Run()
	return nil
}

func Status() {
	log.Debug("getting cluster status")
	currentDockerContext := docker.GetCurrentContext()

	log.Print("current docker context: "+currentDockerContext.Name, " ("+currentDockerContext.DockerEndpoint+")")

	switch config.C.Clusters.Provider {
	case config.ClusterProviderKind:
		kind.Status(config.C.Name)
	case config.ClusterProviderColima:
		colima.Status(config.C.Name)
	}
}

func Ensure() {
	log.Debug("ensuring clusters are created")
	switch config.C.Clusters.Provider {
	case config.ClusterProviderKind:
		kind.Create(config.C.Name)
	case config.ClusterProviderColima:
		colima.Create(config.C.Name)
	}
}

func Stop() {
	log.Info("stopping clusters")
	switch config.C.Clusters.Provider {
	case config.ClusterProviderKind:
		kind.Stop(config.C.Name)
	case config.ClusterProviderColima:
		colima.Stop(config.C.Name)
	}

}
