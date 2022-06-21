package platform

import (
	"fmt"
	"runtime"
)

var (
	ConfigDir    string
	Name         string
	Domain       string
	Arch         = runtime.GOARCH
	NumTargets   int
	LagoonSshKey string
)

func ToMap() map[string]string {
	return map[string]string{
		"Name":     Name,
		"Domain":   Domain,
		"Hostname": fmt.Sprintf("%s.%s", Name, Domain),
		"Arch":     Arch,
	}
}

func Hostname() string {
	return fmt.Sprintf("%s.%s", Name, Domain)
}

func TotalClusterNum() int {
	return NumTargets + 1
}

func ControllerClusterName() string {
	return Name + "-controller"
}

func TargetClusterName(targetId int) string {
	return Name + "-target-" + fmt.Sprint(targetId)
}
