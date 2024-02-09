package components

import (
	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
)

func init() {
	Add("mailhog", Component{
		Name:     "mailhog",
		CompType: ComponentTypeLocalReq,
		InstallActions: []action.Action{
			kube.Applyer{
				Info:      "installing mailhog",
				Namespace: "default",
				Force:     true,
				Template:  "mailhog.yml.tmpl",
			},
		},
	})
}
