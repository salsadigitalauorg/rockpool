package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/yusufhm/rockpool/internal"
)

func CreateCluster(s *State, cn string) {
	res, err := exec.Command("k3d", "cluster", "list", "-o", "json").Output()
	if err != nil {
		fmt.Printf("unable to get cluster list: %s\n", err)
		os.Exit(1)
	}

	clusters := []Cluster{}
	err = json.Unmarshal(res, &clusters)
	if err != nil {
		fmt.Printf("unable to parse cluster list: %s\n", err)
		os.Exit(1)
	}

	for _, c := range clusters {
		if c.Name == cn {
			fmt.Printf("%s cluster already exists\n", cn)
			return
		}
	}

	k3sArgs := []string{"--k3s-arg", "--disable=traefik@server:0", "-p", "80:80@loadbalancer", "-p", "443:443@loadbalancer", "--agents", "2"}
	cmdArgs := []string{"cluster", "create", "--kubeconfig-update-default=false", "--image=rancher/k3s:v1.21.11-k3s1"}
	cmdArgs = append(cmdArgs, k3sArgs...)
	cmdArgs = append(cmdArgs, cn)
	cmd := exec.Command(s.BinaryPaths["k3d"], cmdArgs...)
	fmt.Printf("command to create cluster: %+v\n", cmd)

	err = internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Printf("unable to create cluster: %s", err)
		os.Exit(1)
	}
}

func GetClusterKubeConfigPath(s *State, cn string) {
	out, err := exec.Command(s.BinaryPaths["k3d"], "kubeconfig", "write", cn).CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		fmt.Printf("unable to get kubeconfig: %s\n", err)
	}
	s.Kubeconfig = strings.Trim(string(out), "\n")
}

func ClusterVersion(s *State) {
	cmd := exec.Command("kubectl", "--kubeconfig", s.Kubeconfig, "version")
	err := internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Printf("could not get cluster version: %s\n", err)
	}
}
