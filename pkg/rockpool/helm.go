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
		fmt.Printf("[%s] unable to get list of helm releases: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	releases := []HelmRelease{}
	err = json.Unmarshal(out, &releases)
	if err != nil {
		fmt.Printf("[%s] unable to parse helm releases: %s\n", cn, err)
		os.Exit(1)
	}
	r.State.HelmReleases[cn] = releases
}

func (r *Rockpool) HelmInstallOrUpgrade(cn string, ns string, releaseName string, chartName string, args []string) ([]byte, error) {
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
				fmt.Printf("[%s] helm release %s is already installed\n", cn, releaseName)
				return nil, nil
			}
		}
	} else {
		fmt.Printf("[%s] upgrading helm release %s\n", cn, releaseName)
	}

	// New install.
	if !upgrade {
		fmt.Printf("[%s] installing helm release %s\n", cn, releaseName)
	}

	cmd := r.Helm(cn, ns, "upgrade", "--install", releaseName, chartName)
	cmd.Args = append(cmd.Args, args...)
	fmt.Printf("[%s] command for %s helm release: %v\n", cn, releaseName, cmd)
	return cmd.Output()
}
