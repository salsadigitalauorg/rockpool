package components

import (
	log "github.com/sirupsen/logrus"

	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/clusters"
	"github.com/salsadigitalauorg/rockpool/pkg/config"
)

var Registry = map[string]Component{}
var List = []string{}

func Add(name string, comp Component) {
	Registry[name] = comp
	List = append(List, name)
}

func VerifyRequirements() {
	if config.C.Clusters.Provider != "" {
		clusters.Exist()
	}
}

func Install(name string) {
	chain := action.Chain{}
	comp, ok := Registry[name]
	if !ok {
		log.WithField("component", name).Fatal("Component not found")
	}

	VerifyRequirements()
	for _, action := range comp.InstallActions {
		chain.Add(action)
	}
	chain.Run()
}
