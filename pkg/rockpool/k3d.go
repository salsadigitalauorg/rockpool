package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/salsadigitalauorg/rockpool/internal"
	"github.com/salsadigitalauorg/rockpool/pkg/docker"
)

var registryName = "rockpool-registry"
var registryNameFull = "k3d-rockpool-registry"

func (k3 *K3d) RegistryList() {
	k3.Registries = []Registry{}
	res, err := exec.Command("k3d", "registry", "list", "-o", "json").Output()
	if err != nil {
		fmt.Printf("[rockpool] unable to get registry list: %s\n", err)
		os.Exit(1)
	}

	err = json.Unmarshal(res, &k3.Registries)
	if err != nil {
		fmt.Printf("[rockpool] unable to parse registry list: %s\n", err)
		os.Exit(1)
	}
}

func (k3 *K3d) RegistryGet() {
	k3.RegistryList()
	for _, reg := range k3.Registries {
		if reg.Name == registryNameFull {
			k3.Registry = reg
			break
		}
	}
}

func (k3 *K3d) RegistryCreate() {
	k3.RegistryGet()
	if k3.Registry.Name == registryNameFull {
		fmt.Println("[rockpool] registry already exists")
		return
	}

	fmt.Println("[rockpool] creating registry")
	_, err := exec.Command("k3d", "registry", "create", registryName, "--port", "5111").Output()
	if err != nil {
		fmt.Println("[rockpool] unable to create registry: ", err)
		os.Exit(1)
	}

	// Configure registry to enable proxy.
	regCfgFile := "/etc/docker/registry/config.yml"
	proxyLine := "proxy:\n  remoteurl: https://registry-1.docker.io"
	proxyLineCmdStr := fmt.Sprintf("echo '%s' >> "+regCfgFile, proxyLine)

	var registryConfig []byte
	done := false
	retries := 12
	for !done && retries > 0 {
		registryConfig, err = docker.Exec(registryNameFull, "cat "+regCfgFile)
		if err != nil {
			fmt.Println("[rockpool] unable to find registry container:", internal.GetCmdStdErr(err))
			time.Sleep(5 * time.Second)
			retries--
			continue
		}
		done = true
	}
	if err != nil {
		fmt.Println("[rockpool] unable to find registry container:", internal.GetCmdStdErr(err))
	}

	if !strings.Contains(string(registryConfig), proxyLine) {
		fmt.Println("[rockpool] adding registry proxy config")
		_, err := docker.Exec(registryNameFull, proxyLineCmdStr)
		if err != nil {
			fmt.Println("[rockpool] error adding registry proxy config:", internal.GetCmdStdErr(err))
			os.Exit(1)
		}
		docker.Restart(registryNameFull)
	}
}

func (k3 *K3d) RegistryRenderConfig() {
	_, err := k3.Templates.Render("registries.yaml", nil, "")
	if err != nil {
		panic(err)
	}
}

func (k3 *K3d) RegistryStop() {
	k3.RegistryGet()
	if k3.Registry.Name != registryNameFull {
		return
	}
	fmt.Println("[rockpool] stopping registry")
	_, err := docker.Stop(k3.Registry.Name)
	if err != nil {
		fmt.Println("[rockpool] error stopping registry:", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	fmt.Println("[rockpool] stopped registry")
}

func (k3 *K3d) RegistryStart() {
	fmt.Println("[rockpool] starting registry")
	_, err := docker.Start(registryNameFull)
	if err != nil {
		fmt.Println("[rockpool] error starting registry:", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	fmt.Println("[rockpool] started registry")
}

func (k3 *K3d) RegistryDelete() {
	k3.RegistryGet()
	if k3.Registry.Name != registryNameFull {
		return
	}
	fmt.Println("[rockpool] deleting registry")
	_, err := exec.Command("k3d", "registry", "delete", k3.Registry.Name).Output()
	if err != nil {
		fmt.Println("[rockpool] unable to delete registry: ", err)
		os.Exit(1)
	}
	fmt.Println("[rockpool] deleted registry")
}

func (k3 *K3d) ClusterFetchAll() ClusterList {
	var cl ClusterList
	res, err := exec.Command("k3d", "cluster", "list", "-o", "json").Output()
	if err != nil {
		fmt.Printf("[rockpool] unable to get cluster list: %s\n", internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	err = json.Unmarshal(res, &cl)
	if err != nil {
		fmt.Printf("[rockpool] unable to parse cluster list: %s\n", err)
		os.Exit(1)
	}
	return cl
}

func (k3 *K3d) ClusterExists(clusterName string) (bool, Cluster) {
	for _, c := range k3.Clusters {
		if c.Name == clusterName {
			return true, c
		}
	}
	return false, Cluster{}
}

func (k3 *K3d) ClusterFetch() {
	for _, c := range k3.ClusterFetchAll() {
		if !strings.HasPrefix(c.Name, k3.PlatformName) {
			continue
		}
		// Skip if already present.
		if exists, _ := k3.ClusterExists(c.Name); exists {
			continue
		}
		k3.Clusters = append(k3.Clusters, c)
	}
}

func (k3 *K3d) ClusterIsRunning(clusterName string) bool {
	k3.ClusterFetch()
	for _, c := range k3.Clusters {
		if c.Name != clusterName {
			continue
		}
		return c.AgentsCount == c.AgentsRunning && c.ServersCount == c.ServersRunning
	}
	return false
}

func (k3 *K3d) ClusterCreate(cn string, isController bool) {
	k3.ClusterFetch()
	if exists, _ := k3.ClusterExists(cn); exists && k3.ClusterIsRunning(cn) {
		fmt.Printf("[%s] cluster already exists\n", cn)
		return
	} else if exists {
		fmt.Printf("[%s] cluster already exists, but is stopped; starting now\n", cn)
		k3.ClusterStart(cn)
		return
	}

	k3sArgs := []string{"--k3s-arg", "--disable=traefik@server:0"}
	cmdArgs := []string{
		"cluster", "create", "--kubeconfig-update-default=false",
		"--image=ghcr.io/salsadigitalauorg/rockpool/k3s:latest",
		"--agents", "1", "--network", "k3d-rockpool",
		"--registry-use", registryName + ":5000",
		"--registry-config", fmt.Sprintf("%s/registries.yaml", k3.Templates.RenderedPath(false)),
	}

	if isController {
		cmdArgs = append(cmdArgs,
			"--port", "80:80@loadbalancer",
			"--port", "443:443@loadbalancer",
			"--port", "2022:22@loadbalancer",
			// Required for cross-cluster amqp.
			"--port", "5672:5672@loadbalancer",
			"--port", "6153:6153/udp@loadbalancer",
			"--port", "6153:6153/tcp@loadbalancer",
		)
	} else { // Target cluster exposed ports.
		cmdArgs = append(cmdArgs,
			// Expose arbitrary ports for ingress-nginx.
			"--port", "80@loadbalancer",
			"--port", "443@loadbalancer",
		)
	}

	cmdArgs = append(cmdArgs, k3sArgs...)
	cmdArgs = append(cmdArgs, cn)
	cmd := exec.Command("k3d", cmdArgs...)

	fmt.Printf("[%s] creating cluster: %s\n", cn, cmd)

	_, err := cmd.Output()
	if err != nil {
		fmt.Printf("[%s] unable to create cluster: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	fmt.Printf("[%s] created cluster\n", cn)
	k3.ClusterFetch()
}

func (k3 *K3d) ClusterStart(cn string) {
	if exists, _ := k3.ClusterExists(cn); !exists {
		fmt.Printf("[%s] cluster does not exist\n", cn)
		return
	}
	fmt.Printf("[%s] starting cluster\n", cn)
	// _, err := exec.Command(r.State.BinaryPaths["k3d"], "cluster", "start", cn).Output()
	_, err := internal.RunCmdWithProgress(exec.Command("k3d", "cluster", "start", cn))
	if err != nil {
		fmt.Printf("[%s] unable to start cluster: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	k3.ClusterFetch()
	fmt.Printf("[%s] started cluster\n", cn)
}

func (k3 *K3d) ClusterStop(cn string) {
	if exists, _ := k3.ClusterExists(cn); !exists {
		fmt.Printf("[%s] cluster does not exist\n", cn)
		return
	}
	fmt.Printf("[%s] stopping cluster\n", cn)
	_, err := internal.RunCmdWithProgress(exec.Command("k3d", "cluster", "stop", cn))
	if err != nil {
		fmt.Printf("[%s] unable to stop cluster: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	k3.ClusterFetch()
	fmt.Printf("[%s] stopped cluster\n", cn)
}

func (k3 *K3d) ClusterRestart(cn string) {
	k3.ClusterStop(cn)
	k3.ClusterStart(cn)
}

func (k3 *K3d) ClusterDelete(cn string) {
	defer k3.WgDone()
	if exists, _ := k3.ClusterExists(cn); !exists {
		return
	}
	k3.ClusterStop(cn)
	fmt.Printf("[%s] deleting cluster\n", cn)
	_, err := exec.Command("k3d", "cluster", "delete", cn).Output()
	if err != nil {
		fmt.Printf("[%s] unable to delete cluster: %s\n", cn, err)
		os.Exit(1)
	}
	k3.ClusterFetch()
	fmt.Printf("[%s] deleted cluster\n", cn)
}

func (k3 *K3d) WriteKubeConfig(cn string) {
	fmt.Printf("[%s] writing kubeconfig\n", cn)
	_, err := exec.Command("k3d", "kubeconfig", "write", cn).CombinedOutput()
	if err != nil {
		panic(fmt.Sprintln("unable to write kubeconfig:", err))
	}
}

func (r *Rockpool) ClusterVersion(cn string) {
	_, err := r.KubeCtl(cn, "", "version").Output()
	if err != nil {
		fmt.Printf("[%s] could not get cluster version: %s\n", cn, err)
	}
}
