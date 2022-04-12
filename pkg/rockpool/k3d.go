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

func (r *Rockpool) FetchClusters() {
	var allK3dCl ClusterList
	allK3dCl.Get()
	for _, c := range allK3dCl {
		if !strings.HasPrefix(c.Name, r.ClusterName) {
			continue
		}
		r.Clusters = append(r.Clusters, c)
	}
}

func (r *Rockpool) CreateCluster(cn string) {
	r.FetchClusters()
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
		"--agents", "1",
	}

	if strings.HasSuffix(cn, "-controller") {
		cmdArgs = append(cmdArgs,
			"-p", "80:80@loadbalancer",
			"-p", "443:443@loadbalancer",
			"-p", "2022:22@loadbalancer",
		)
	}

	cmdArgs = append(cmdArgs, k3sArgs...)
	cmdArgs = append(cmdArgs, cn)
	cmd := exec.Command(r.State.BinaryPaths["k3d"], cmdArgs...)
	fmt.Printf("command to create cluster: %+v\n", cmd)

	err := internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Printf("unable to create cluster: %s", err)
		os.Exit(1)
	}
	r.FetchClusters()
}

func (r *Rockpool) StartCluster(cn string) {
	if exists, _ := r.State.Clusters.ClusterExists(cn); !exists {
		fmt.Printf("%s cluster does not exist\n", cn)
		os.Exit(1)
	}
	cmd := exec.Command(r.State.BinaryPaths["k3d"], "cluster", "start", cn)
	err := internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Printf("unable to start cluster: %s", err)
		os.Exit(1)
	}
	r.FetchClusters()
}

func (r *Rockpool) StopCluster(cn string) {
	if exists, _ := r.State.Clusters.ClusterExists(cn); !exists {
		fmt.Printf("%s cluster does not exist\n", cn)
		os.Exit(1)
	}
	cmd := exec.Command(r.State.BinaryPaths["k3d"], "cluster", "stop", cn)
	err := internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Printf("unable to stop cluster: %s", err)
		os.Exit(1)
	}
	r.FetchClusters()
	if r.wg != nil {
		r.wg.Done()
	}
}

func (r *Rockpool) DeleteCluster(cn string) {
	r.StopCluster(cn)
	cmd := exec.Command(r.State.BinaryPaths["k3d"], "cluster", "delete", cn)
	err := internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Printf("unable to delete cluster: %s", err)
		os.Exit(1)
	}
	r.FetchClusters()
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
	err := internal.RunCmdWithProgress(r.KubeCtl(cn, "", "version"))
	if err != nil {
		fmt.Printf("could not get cluster version: %s\n", err)
	}
}
