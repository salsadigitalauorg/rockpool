package clusters

import (
	"github.com/salsadigitalauorg/rockpool/pkg/action"
)

type ClusterRole string

const (
	ClusterRoleTools      ClusterRole = "tools"
	ClusterRoleController ClusterRole = "controller"
	ClusterRoleRemote     ClusterRole = "remote"
)

type ClusterNode struct {
	Status string
	Roles  []ClusterRole
}

type ClusterProvider interface {
	GetRequiredBinaries() []action.BinaryExists
	Exist(string) bool
	Clusters() []string
	ClusterNodes(string) map[string]ClusterNode
	Status(string)
	Create(string)
	Start(string)
	Stop(string)
	Delete(string)
}
