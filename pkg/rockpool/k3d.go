package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

func (r *Rockpool) FetchClusters() {
	var allK3dCl ClusterList
	allK3dCl.Get()
	for _, c := range allK3dCl {
		if !strings.HasPrefix(c.Name, r.ClusterName) {
			continue
		}
		if exists, _ := r.Clusters.ClusterExists(c.Name); exists {
			continue
		}
		r.Clusters = append(r.Clusters, c)
	}
}

func (r *Rockpool) CreateCluster(cn string) {
	if exists, cs := r.State.Clusters.ClusterExists(cn); exists && cs.IsRunning() {
		fmt.Printf("%s cluster already exists\n", cn)
		r.WgDone()
		return
	} else if exists {
		fmt.Printf("%s cluster already exists, but is stopped; starting now\n", cn)
		r.StartCluster(cn)
		r.WgDone()
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

	fmt.Printf("creating cluster %s...", cn)
	fmt.Println("command to create cluster:", cmd)

	_, err := cmd.Output()
	if err != nil {
		fmt.Println("unable to create cluster:", err)
		os.Exit(1)
	}
	fmt.Println("created cluster", cn)
	r.WgDone()
}

func (r *Rockpool) StartCluster(cn string) {
	if exists, _ := r.State.Clusters.ClusterExists(cn); !exists {
		fmt.Printf("%s cluster does not exist\n", cn)
		os.Exit(1)
	}
	fmt.Printf("starting cluster %s...", cn)
	_, err := exec.Command(r.State.BinaryPaths["k3d"], "cluster", "start", cn).Output()
	if err != nil {
		fmt.Println("unable to start cluster:", err)
		os.Exit(1)
	}
	r.FetchClusters()
	fmt.Println("started cluster", cn)
}

func (r *Rockpool) StopCluster(cn string) {
	if exists, _ := r.State.Clusters.ClusterExists(cn); !exists {
		fmt.Printf("%s cluster does not exist\n", cn)
		r.WgDone()
		return
	}
	fmt.Printf("stopping cluster %s...", cn)
	_, err := exec.Command(r.State.BinaryPaths["k3d"], "cluster", "stop", cn).Output()
	if err != nil {
		fmt.Printf("unable to stop cluster: %s", err)
		os.Exit(1)
	}
	r.FetchClusters()
	r.WgDone()
	fmt.Println("stopped cluster", cn)
}

func (r *Rockpool) RestartCluster(cn string) {
	r.StopCluster(cn)
	r.StartCluster(cn)
}

func (r *Rockpool) DeleteCluster(cn string) {
	r.wg.Add(1)
	r.StopCluster(cn)
	fmt.Printf("deleting cluster %s...", cn)
	_, err := exec.Command(r.State.BinaryPaths["k3d"], "cluster", "delete", cn).Output()
	if err != nil {
		fmt.Printf("unable to delete cluster: %s", err)
		os.Exit(1)
	}
	r.FetchClusters()
	r.WgDone()
	fmt.Println("deleted cluster", cn)
}

func (r *Rockpool) GetClusterKubeConfigPath(cn string) {
	out, err := exec.Command(r.State.BinaryPaths["k3d"], "kubeconfig", "write", cn).CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		fmt.Printf("unable to get kubeconfig: %s\n", err)
	}
	r.State.Kubeconfig[cn] = strings.Trim(string(out), "\n")
}

func (r *Rockpool) ClusterVersion(cn string) {
	_, err := r.KubeCtl(cn, "", "version").Output()
	if err != nil {
		fmt.Printf("could not get cluster version: %s\n", err)
	}
}
