package clusters

import (
	log "github.com/sirupsen/logrus"

	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/config"
	"github.com/salsadigitalauorg/rockpool/pkg/docker"
)

var requiredBinaries = []action.BinaryExists{
	{Bin: "docker", VersionArgs: []string{"--format", "json"}},
	{Bin: "kubectl", VersionArgs: []string{"--client"}},
	{Bin: "helm"},
}

var clusterProvider ClusterProvider

func Provider() ClusterProvider {
	if clusterProvider != nil {
		return clusterProvider
	}
	switch config.C.Clusters.Provider {
	case config.ClusterProviderKind:
		clusterProvider = &kindcp
	}
	return clusterProvider
}

func VerifyRequirements() error {
	requiredBinaries = append(requiredBinaries, Provider().GetRequiredBinaries()...)

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
	log.Debug("getting docker provider")
	dockerProvider := docker.GetProvider()
	log.Print("docker provider: " + dockerProvider)

	log.Debug("getting cluster status")
	currentDockerContext := docker.GetCurrentContext()

	log.Print("current docker context: "+currentDockerContext.Name, " ("+currentDockerContext.DockerEndpoint+")")

	Provider().Status(config.C.Name)
}

func Exist() bool {
	return Provider().Exist(config.C.Name)
}

func Ensure() {
	log.Debug("ensuring clusters are created")
	Provider().Create(config.C.Name)
}

func Stop() {
	log.Info("stopping clusters")
	Provider().Stop(config.C.Name)
}

func Delete() {
	log.Info("deleting clusters")
	Provider().Delete(config.C.Name)
}
