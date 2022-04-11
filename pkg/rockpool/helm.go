package rockpool

import (
	"fmt"
	"os/exec"

	"github.com/yusufhm/rockpool/internal"
)

func helmAddRepo(name string, url string) error {
	cmd := exec.Command("helm", "repo", "add", name, url)
	return internal.RunCmdWithProgress(cmd)
}

func helmInstallOrUpgrade(s *State, releaseName string, chartName string, args []string) error {
	for _, r := range s.HelmReleases {
		if r.Name == releaseName {
			fmt.Printf("helm release %s is already installed\n", releaseName)
			return nil
		}
	}

	cmd := exec.Command("helm", "--kubeconfig", s.Kubeconfig)
	cmd.Args = append(cmd.Args, "install")
	cmd.Args = append(cmd.Args, releaseName, chartName)
	cmd.Args = append(cmd.Args, args...)
	fmt.Printf("command for %s helm release: %v\n", releaseName, cmd)
	return internal.RunCmdWithProgress(cmd)
}
