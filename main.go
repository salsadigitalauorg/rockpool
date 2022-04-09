package main

import (
	"fmt"
	"os"
	"os/exec"
)

var binaryPaths map[string]string
var kubeconfig string
var helmReleases []HelmRelease

func main() {
	verifyReqs()
	fmt.Println()
	createCluster("rockpool")
	fmt.Println()
	kubeconfig = getClusterKubeConfigPath("rockpool")
	clusterVersion()
	fmt.Println()
	helmList()
	installIngressNginx()
	installHarbor("harbor.lagoon.rockpool.k3d.local", "pass")
}

func verifyReqs() {
	binaries := []string{"k3d", "docker", "kubectl", "helm", "lagoon"}
	missing := []string{}
	binaryPaths = map[string]string{}
	for _, b := range binaries {
		path, err := exec.LookPath(b)
		if err != nil {
			missing = append(missing, fmt.Sprintf("could not find %s; please ensure it is installed before", b))
			continue
		}
		fmt.Printf("%s is available at %s\n", b, path)
		binaryPaths[b] = path
	}
	for _, m := range missing {
		fmt.Printf(m)
	}
	if len(missing) > 0 {
		fmt.Println("some requirements were not met; please review above")
		os.Exit(1)
	}
}

func clusterVersion() {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "version")
	err := runCmdWithProgress(cmd)
	if err != nil {
		fmt.Printf("could not get cluster version: %s\n", err)
	}
}
