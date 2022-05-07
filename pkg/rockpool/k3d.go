package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/yusufhm/rockpool/internal"
)

func (cr *Cluster) IsRunning() bool {
	return cr.AgentsCount == cr.AgentsRunning && cr.ServersCount == cr.ServersRunning
}

func (cl *ClusterList) Get() {
	res, err := exec.Command("k3d", "cluster", "list", "-o", "json").Output()
	if err != nil {
		fmt.Printf("[rockpool] unable to get cluster list: %s\n", err)
		os.Exit(1)
	}

	err = json.Unmarshal(res, cl)
	if err != nil {
		fmt.Printf("[rockpool] unable to parse cluster list: %s\n", err)
		os.Exit(1)
	}
}

func (cl *ClusterList) ClusterExists(cn string) (bool, Cluster) {
	for _, c := range *cl {
		if c.Name == cn {
			return true, c
		}
	}
	return false, Cluster{}
}

func (r *Rockpool) FetchRegistry() {
	var allRegistries []Registry
	res, err := exec.Command("k3d", "registry", "list", "-o", "json").Output()
	if err != nil {
		fmt.Printf("[rockpool] unable to get registry list: %s\n", err)
		os.Exit(1)
	}

	err = json.Unmarshal(res, &allRegistries)
	if err != nil {
		fmt.Printf("[rockpool] unable to parse registry list: %s\n", err)
		os.Exit(1)
	}

	for _, reg := range allRegistries {
		if reg.Name == "k3d-rockpool-registry" {
			r.Registry = reg
			break
		}
	}

}

func (r *Rockpool) CreateRegistry() {
	r.FetchRegistry()
	if r.Registry.Name == "k3d-rockpool-registry" {
		fmt.Println("[rockpool] registry already exists")
		return
	}

	fmt.Println("[rockpool] creating registry")
	_, err := exec.Command("k3d", "registry", "create", "rockpool-registry").Output()
	if err != nil {
		fmt.Println("[rockpool] unable to create registry: ", err)
		os.Exit(1)
	}
	fmt.Println("[rockpool] created registry")
}

func (r *Rockpool) CreateCluster(cn string) {
	if exists, cs := r.State.Clusters.ClusterExists(cn); exists && cs.IsRunning() {
		fmt.Printf("[%s] cluster already exists\n", cn)
		return
	} else if exists {
		fmt.Printf("[%s] cluster already exists, but is stopped; starting now\n", cn)
		r.StartCluster(cn)
		return
	}

	k3sArgs := []string{"--k3s-arg", "--disable=traefik@server:0"}
	cmdArgs := []string{
		"cluster", "create", "--kubeconfig-update-default=false",
		"--image=ghcr.io/yusufhm/rockpool/k3s:latest",
		"--agents", "1", "--network", "k3d-rockpool",
		"--registry-use", "k3d-rockpool-registry",
	}

	if cn == r.ControllerClusterName() {
		cmdArgs = append(cmdArgs,
			"-p", "80:80@loadbalancer",
			"-p", "443:443@loadbalancer",
			"-p", "2022:22@loadbalancer",
			"-p", "5672:5672@loadbalancer",
		)
	}

	cmdArgs = append(cmdArgs, k3sArgs...)
	cmdArgs = append(cmdArgs, cn)
	cmd := exec.Command(r.GetBinaryPath("k3d"), cmdArgs...)

	fmt.Printf("[%s] creating cluster: %s\n", cn, cmd)

	_, err := cmd.Output()
	if err != nil {
		fmt.Printf("[%s] unable to create cluster: %s\n", cn, err)
		os.Exit(1)
	}
	fmt.Printf("[%s] created cluster\n", cn)
}

func (r *Rockpool) StartCluster(cn string) {
	if exists, _ := r.State.Clusters.ClusterExists(cn); !exists {
		fmt.Printf("[%s] cluster does not exist\n", cn)
		return
	}
	fmt.Printf("[%s] starting cluster\n", cn)
	// _, err := exec.Command(r.State.BinaryPaths["k3d"], "cluster", "start", cn).Output()
	_, err := internal.RunCmdWithProgress(exec.Command(r.GetBinaryPath("k3d"), "cluster", "start", cn))
	if err != nil {
		fmt.Printf("[%s] unable to start cluster: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	r.FetchClusters()
	fmt.Printf("[%s] started cluster\n", cn)
	r.AddHarborHostEntries(cn)
	if cn != r.ControllerClusterName() {
		r.ConfigureTargetCoreDNS(cn)
	}
}

func (r *Rockpool) StopCluster(cn string) {
	if exists, _ := r.State.Clusters.ClusterExists(cn); !exists {
		fmt.Printf("[%s] cluster does not exist\n", cn)
		return
	}
	fmt.Printf("[%s] stopping cluster\n", cn)
	_, err := internal.RunCmdWithProgress(exec.Command(r.GetBinaryPath("k3d"), "cluster", "stop", cn))
	if err != nil {
		fmt.Printf("[%s] unable to stop cluster: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	r.FetchClusters()
	fmt.Printf("[%s] stopped cluster\n", cn)
}

func (r *Rockpool) RestartCluster(cn string) {
	r.StopCluster(cn)
	r.StartCluster(cn)
}

func (r *Rockpool) DeleteCluster(cn string) {
	defer r.WgDone()
	if exists, _ := r.State.Clusters.ClusterExists(cn); !exists {
		return
	}
	r.StopCluster(cn)
	fmt.Printf("[%s] deleting cluster\n", cn)
	_, err := exec.Command(r.GetBinaryPath("k3d"), "cluster", "delete", cn).Output()
	if err != nil {
		fmt.Printf("[%s] unable to delete cluster: %s\n", cn, err)
		os.Exit(1)
	}
	r.FetchClusters()
	fmt.Printf("[%s] deleted cluster\n", cn)
}

func (r *Rockpool) GetClusterKubeConfigPath(cn string) {
	out, err := exec.Command(r.GetBinaryPath("k3d"), "kubeconfig", "write", cn).CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		fmt.Printf("[%s] unable to get kubeconfig: %s\n", cn, err)
	}
	r.State.Kubeconfig.Store(cn, strings.Trim(string(out), "\n"))
}

func (r *Rockpool) ClusterVersion(cn string) {
	_, err := r.KubeCtl(cn, "", "version").Output()
	if err != nil {
		fmt.Printf("[%s] could not get cluster version: %s\n", cn, err)
	}
}
