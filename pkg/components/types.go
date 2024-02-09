package components

import "github.com/salsadigitalauorg/rockpool/pkg/action"

type ComponentType string

const (
	ComponentTypeLocalReq     = "LocalRequirement"
	ComponentTypeLagoonReq    = "LagoonRequirement"
	ComponentTypeLagoonCore   = "LagoonCore"
	ComponentTypeLagoonRemote = "LagoonRemote"
)

type Component struct {
	Name           string
	CompType       ComponentType
	InstallActions []action.Action
}
