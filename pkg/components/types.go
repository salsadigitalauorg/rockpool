package components

import "github.com/salsadigitalauorg/rockpool/pkg/action"

type Component struct {
	Name           string
	InstallActions []action.Action
}
