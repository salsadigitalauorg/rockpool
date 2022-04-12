package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/yusufhm/rockpool/internal"
)

func (r *Rockpool) Helm(cn string, ns string, args ...string) *exec.Cmd {
	cmd := exec.Command("helm", "--kubeconfig", r.State.Kubeconfig[cn])
	if ns != "" {
		cmd.Args = append(cmd.Args, "--namespace", ns)
	}
	cmd.Args = append(cmd.Args, args...)
	return cmd
}

func (r *Rockpool) HelmList(cn string) {
	cmd := r.Helm(cn, "", "list", "--all-namespaces", "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		fmt.Println(string(out))
		fmt.Println("unable to get list of helm releases: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	releases := []HelmRelease{}
	err = json.Unmarshal(out, &releases)
	if err != nil {
		fmt.Println("unable to parse helm releases: ", err)
		os.Exit(1)
	}
	r.State.HelmReleases[cn] = releases
}

func (r *Rockpool) HelmInstallOrUpgrade(cn string, ns string, releaseName string, chartName string, args []string) error {
	upgrade := false
	for _, u := range r.Config.UpgradeComponents {
		if u == releaseName {
			upgrade = true
			break
		}
	}
	if !upgrade {
		for _, r := range r.State.HelmReleases[cn] {
			if r.Name == releaseName {
				fmt.Printf("helm release %s is already installed\n", releaseName)
				return nil
			}
		}
	} else {
		fmt.Println("upgrading helm release ", releaseName)
	}

	cmd := r.Helm(cn, ns, "upgrade", "--install", releaseName, chartName)
	cmd.Args = append(cmd.Args, args...)
	fmt.Printf("command for %s helm release: %v\n", releaseName, cmd)
	return internal.RunCmdWithProgress(cmd)
}
