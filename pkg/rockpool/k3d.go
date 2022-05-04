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
		fmt.Printf("unable to get cluster list: %s\n", err)
		os.Exit(1)
	}

	err = json.Unmarshal(res, cl)
	if err != nil {
		fmt.Printf("unable to parse cluster list: %s\n", err)
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
		fmt.Printf("unable to get registry list: %s\n", err)
		os.Exit(1)
	}

	err = json.Unmarshal(res, &allRegistries)
	if err != nil {
		fmt.Printf("unable to parse registry list: %s\n", err)
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
		fmt.Println("registry already exists")
		return
	}

	fmt.Println("creating registry...")
	_, err := exec.Command("k3d", "registry", "create", "rockpool-registry").Output()
	if err != nil {
		fmt.Println("unable to create registry: ", err)
		os.Exit(1)
	}
	fmt.Println("created registry")
}

func (r *Rockpool) CreateCluster(cn string) {
	if exists, cs := r.State.Clusters.ClusterExists(cn); exists && cs.IsRunning() {
		fmt.Printf("%s cluster already exists\n", cn)
		return
	} else if exists {
		fmt.Printf("%s cluster already exists, but is stopped; starting now\n", cn)
		r.StartCluster(cn)
		return
	}

	k3sArgs := []string{"--k3s-arg", "--disable=traefik@server:0"}
	cmdArgs := []string{
		"cluster", "create", "--kubeconfig-update-default=false",
		"--image=rancher/k3s:v1.21.11-k3s1",
		"--agents", "1", "--network", "k3d-rockpool",
		"--registry-use", "k3d-rockpool-registry",
	}

	if strings.HasSuffix(cn, "-controller") {
		cmdArgs = append(cmdArgs,
			"-p", "80:80@loadbalancer",
			"-p", "443:443@loadbalancer",
			"-p", "2022:22@loadbalancer",
			"-p", "5672:5672@loadbalancer",
		)
	}

	cmdArgs = append(cmdArgs, k3sArgs...)
	cmdArgs = append(cmdArgs, cn)
	cmd := exec.Command(r.State.BinaryPaths["k3d"], cmdArgs...)

	fmt.Printf("creating cluster %s...\n", cn)
	fmt.Println("command to create cluster:", cmd)

	_, err := cmd.Output()
	if err != nil {
		fmt.Println("unable to create cluster:", err)
		os.Exit(1)
	}
	fmt.Println("created cluster", cn)
}

func (r *Rockpool) StartCluster(cn string) {
	if exists, _ := r.State.Clusters.ClusterExists(cn); !exists {
		fmt.Printf("%s cluster does not exist\n", cn)
		return
	}
	fmt.Printf("starting cluster %s...\n", cn)
	// _, err := exec.Command(r.State.BinaryPaths["k3d"], "cluster", "start", cn).Output()
	_, err := internal.RunCmdWithProgress(exec.Command(r.State.BinaryPaths["k3d"], "cluster", "start", cn))
	if err != nil {
		fmt.Println("unable to start cluster:", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	r.FetchClusters()
	fmt.Println("started cluster", cn)
	r.AddHarborHostEntries(cn)
}

func (r *Rockpool) StopCluster(cn string) {
	if exists, _ := r.State.Clusters.ClusterExists(cn); !exists {
		fmt.Printf("%s cluster does not exist\n", cn)
		return
	}
	fmt.Printf("stopping cluster %s...\n", cn)
	_, err := internal.RunCmdWithProgress(exec.Command(r.State.BinaryPaths["k3d"], "cluster", "stop", cn))
	if err != nil {
		fmt.Println("unable to stop cluster:", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	r.FetchClusters()
	fmt.Println("stopped cluster", cn)
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
	fmt.Printf("deleting cluster %s...\n", cn)
	_, err := exec.Command(r.State.BinaryPaths["k3d"], "cluster", "delete", cn).Output()
	if err != nil {
		fmt.Println("unable to delete cluster:", err)
		os.Exit(1)
	}
	r.FetchClusters()
	fmt.Println("deleted cluster", cn)
}

func (r *Rockpool) GetClusterKubeConfigPath(cn string) {
	out, err := exec.Command(r.State.BinaryPaths["k3d"], "kubeconfig", "write", cn).CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		fmt.Println("unable to get kubeconfig:", err)
	}
	r.State.Kubeconfig[cn] = strings.Trim(string(out), "\n")
}

func (r *Rockpool) ClusterVersion(cn string) {
	_, err := r.KubeCtl(cn, "", "version").Output()
	if err != nil {
		fmt.Printf("could not get cluster version: %s\n", err)
	}
}
