package clusters

import (
	"github.com/salsadigitalauorg/rockpool/pkg/action"
)

type ClusterNode struct {
	Status string
}

type ClusterProvider interface {
	GetRequiredBinaries() []action.BinaryExists
	Exist(string) bool
	Clusters() []string
	ClusterNodes(string) map[string]ClusterNode
	Status(string)
	Create(string)
	Stop(string)
	Delete(string)
}
