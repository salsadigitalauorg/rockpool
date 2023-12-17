package clusters

import "github.com/salsadigitalauorg/rockpool/pkg/action"

type ClusterProvider interface {
	GetRequiredBinaries() []action.BinaryExists
	Exist(string) bool
	List() map[string][]string
	Status(string)
	Create(string)
	Stop(string)
	Delete(string)
}
