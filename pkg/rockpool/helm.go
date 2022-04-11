package rockpool

import (
	"fmt"
	"os/exec"

	"github.com/yusufhm/rockpool/internal"
)

func HelmAddRepo(name string, url string) error {
	cmd := exec.Command("helm", "repo", "add", name, url)
	return internal.RunCmdWithProgress(cmd)
}

func HelmInstallOrUpgrade(s *State, c *Config, releaseName string, chartName string, args []string) error {
	upgrade := false
	for _, u := range c.UpgradeComponents {
		if u == releaseName {
			upgrade = true
			break
		}
	}
	if !upgrade {
		for _, r := range s.HelmReleases {
			if r.Name == releaseName {
				fmt.Printf("helm release %s is already installed\n", releaseName)
				return nil
			}
		}
	}

	cmd := exec.Command(
		"helm", "--kubeconfig", s.Kubeconfig,
		"upgrade", "--install", releaseName, chartName,
	)
	cmd.Args = append(cmd.Args, args...)
	fmt.Printf("command for %s helm release: %v\n", releaseName, cmd)
	return internal.RunCmdWithProgress(cmd)
}
