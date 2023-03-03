package helm

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/salsadigitalauorg/rockpool/internal"
)

// List of Helm releases per cluster.
var Releases sync.Map
var UpgradeComponents []string

func Exec(cn string, ns string, args ...string) *exec.Cmd {
	cmd := exec.Command("helm", "--kubeconfig", internal.KubeconfigPath(cn))
	if ns != "" {
		cmd.Args = append(cmd.Args, "--namespace", ns)
	}
	cmd.Args = append(cmd.Args, args...)
	return cmd
}

func List(cn string) {
	cmd := Exec(cn, "", "list", "--all-namespaces", "--output", "json")
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
	Releases.Store(cn, releases)
}

func GetReleases(key string) []HelmRelease {
	valueIfc, ok := Releases.Load(key)
	if !ok {
		panic(fmt.Sprint("releases not found for ", key))
	}
	val, ok := valueIfc.([]HelmRelease)
	if !ok {
		panic(fmt.Sprint("unable to convert binpath to string for ", valueIfc))
	}
	return val
}

func InstallOrUpgrade(cn string, ns string, releaseName string, chartName string, args []string) ([]byte, error) {
	upgrade := false
	for _, u := range UpgradeComponents {
		if u == "all" || u == releaseName {
			upgrade = true
			break
		}
	}
	if !upgrade {
		for _, r := range GetReleases(cn) {
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

	cmd := Exec(cn, ns, "upgrade", "--install", releaseName, chartName)
	cmd.Args = append(cmd.Args, args...)
	fmt.Printf("[%s] command for %s helm release: %v\n", cn, releaseName, cmd)
	return cmd.Output()
}
