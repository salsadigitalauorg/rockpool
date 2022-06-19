package k3d

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/salsadigitalauorg/rockpool/internal"
	"github.com/salsadigitalauorg/rockpool/pkg/docker"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	"github.com/salsadigitalauorg/rockpool/pkg/templates"
	"github.com/salsadigitalauorg/rockpool/pkg/wg"
)

var registryName = "rockpool-registry"
var registryNameFull = "k3d-rockpool-registry"

var registries []Registry
var Reg Registry

var Clusters ClusterList

func RegistryList() {
	res, err := exec.Command("k3d", "registry", "list", "-o", "json").Output()
	if err != nil {
		fmt.Printf("[rockpool] unable to get registry list: %s\n", err)
		os.Exit(1)
	}

	err = json.Unmarshal(res, &registries)
	if err != nil {
		fmt.Printf("[rockpool] unable to parse registry list: %s\n", err)
		os.Exit(1)
	}
}

func RegistryGet() {
	RegistryList()
	for _, reg := range registries {
		if reg.Name == registryNameFull {
			Reg = reg
			break
		}
	}
}

func RegistryCreate() {
	RegistryGet()
	if Reg.Name == registryNameFull {
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

func RegistryRenderConfig() {
	_, err := templates.Render("registries.yaml", nil, "")
	if err != nil {
		panic(err)
	}
}

func RegistryStop() {
	RegistryGet()
	if Reg.Name != registryNameFull {
		return
	}
	fmt.Println("[rockpool] stopping registry")
	_, err := docker.Stop(Reg.Name)
	if err != nil {
		fmt.Println("[rockpool] error stopping registry:", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	fmt.Println("[rockpool] stopped registry")
}

func RegistryStart() {
	fmt.Println("[rockpool] starting registry")
	_, err := docker.Start(registryNameFull)
	if err != nil {
		fmt.Println("[rockpool] error starting registry:", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	fmt.Println("[rockpool] started registry")
}

func RegistryDelete() {
	RegistryGet()
	if Reg.Name != registryNameFull {
		return
	}
	fmt.Println("[rockpool] deleting registry")
	_, err := exec.Command("k3d", "registry", "delete", Reg.Name).Output()
	if err != nil {
		fmt.Println("[rockpool] unable to delete registry: ", err)
		os.Exit(1)
	}
	fmt.Println("[rockpool] deleted registry")
}

func ClusterFetchAll() ClusterList {
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

func ClusterExists(clusterName string) (bool, Cluster) {
	for _, c := range Clusters {
		if c.Name == clusterName {
			return true, c
		}
	}
	return false, Cluster{}
}

func ClusterFetch() {
	for _, c := range ClusterFetchAll() {
		if !strings.HasPrefix(c.Name, platform.Name) {
			continue
		}
		// Skip if already present.
		if exists, _ := ClusterExists(c.Name); exists {
			continue
		}
		Clusters = append(Clusters, c)
	}
}

func ClusterIsRunning(clusterName string) bool {
	ClusterFetch()
	for _, c := range Clusters {
		if c.Name != clusterName {
			continue
		}
		return c.AgentsCount == c.AgentsRunning && c.ServersCount == c.ServersRunning
	}
	return false
}

func ClusterCreate(cn string, isController bool) {
	ClusterFetch()
	if exists, _ := ClusterExists(cn); exists && ClusterIsRunning(cn) {
		fmt.Printf("[%s] cluster already exists\n", cn)
		return
	} else if exists {
		fmt.Printf("[%s] cluster already exists, but is stopped; starting now\n", cn)
		ClusterStart(cn)
		return
	}

	k3sArgs := []string{"--k3s-arg", "--disable=traefik@server:0"}
	cmdArgs := []string{
		"cluster", "create", "--kubeconfig-update-default=false",
		"--image=ghcr.io/salsadigitalauorg/rockpool/k3s:latest",
		"--agents", "1", "--network", "k3d-rockpool",
		"--registry-use", registryName + ":5000",
		"--registry-config", fmt.Sprintf("%s/registries.yaml", templates.RenderedPath(false)),
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
	ClusterFetch()
}

func ClusterStart(cn string) {
	if exists, _ := ClusterExists(cn); !exists {
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
	ClusterFetch()
	fmt.Printf("[%s] started cluster\n", cn)
}

func ClusterStop(cn string) {
	if exists, _ := ClusterExists(cn); !exists {
		fmt.Printf("[%s] cluster does not exist\n", cn)
		return
	}
	fmt.Printf("[%s] stopping cluster\n", cn)
	_, err := internal.RunCmdWithProgress(exec.Command("k3d", "cluster", "stop", cn))
	if err != nil {
		fmt.Printf("[%s] unable to stop cluster: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	ClusterFetch()
	fmt.Printf("[%s] stopped cluster\n", cn)
}

func ClusterRestart(cn string) {
	ClusterStop(cn)
	ClusterStart(cn)
}

func ClusterDelete(cn string) {
	defer wg.Done()
	if exists, _ := ClusterExists(cn); !exists {
		return
	}
	ClusterStop(cn)
	fmt.Printf("[%s] deleting cluster\n", cn)
	_, err := exec.Command("k3d", "cluster", "delete", cn).Output()
	if err != nil {
		fmt.Printf("[%s] unable to delete cluster: %s\n", cn, err)
		os.Exit(1)
	}
	ClusterFetch()
	fmt.Printf("[%s] deleted cluster\n", cn)
}

func WriteKubeConfig(cn string) {
	fmt.Printf("[%s] writing kubeconfig\n", cn)
	_, err := exec.Command("k3d", "kubeconfig", "write", cn).CombinedOutput()
	if err != nil {
		panic(fmt.Sprintln("unable to write kubeconfig:", err))
	}
}
