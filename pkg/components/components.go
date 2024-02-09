package components

import (
	log "github.com/sirupsen/logrus"

	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/clusters"
	"github.com/salsadigitalauorg/rockpool/pkg/config"
)

// We keep a map of functions that return a Component, so that the logic
// is executed only when the component is actually installed. This is crucial
// for components that require config.C to be initialised.
var Registry = map[string]func() Component{}
var List = []string{}

func Add(name string, compFunc func() Component) {
	Registry[name] = compFunc
	List = append(List, name)
}

func VerifyRequirements() {
	if config.C.Clusters.Provider != "" {
		clusters.Exist()
	}
}

func Install(name string) {
	chain := action.Chain{}
	compFunc, ok := Registry[name]
	if !ok {
		log.WithField("component", name).Fatal("Component not found")
	}

	VerifyRequirements()
	for _, action := range compFunc().InstallActions {
		chain.Add(action)
	}
	chain.Run()
}
