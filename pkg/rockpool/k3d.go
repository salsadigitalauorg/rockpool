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
	if len(*cl) == 0 {
		cl.Get()
	}
	for _, c := range *cl {
		if c.Name == cn {
			return true, c
		}
	}
	return false, Cluster{}
}

func CreateCluster(s *State, cn string) {
	if exists, cs := s.Clusters.ClusterExists(cn); exists && cs.IsRunning() {
		fmt.Printf("%s cluster already exists\n", cn)
		return
	} else if exists {
		fmt.Printf("%s cluster already exists, but is stopped; starting now\n", cn)
		StartCluster(s, cn)
		return
	}

	k3sArgs := []string{
		"--k3s-arg", "--disable=traefik@server:0", "-p", "80:80@loadbalancer",
		"-p", "443:443@loadbalancer", "-p", "2022:22@loadbalancer",
		"--agents", "2",
	}
	cmdArgs := []string{"cluster", "create", "--kubeconfig-update-default=false", "--image=rancher/k3s:v1.21.11-k3s1"}
	cmdArgs = append(cmdArgs, k3sArgs...)
	cmdArgs = append(cmdArgs, cn)
	cmd := exec.Command(s.BinaryPaths["k3d"], cmdArgs...)
	fmt.Printf("command to create cluster: %+v\n", cmd)

	err := internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Printf("unable to create cluster: %s", err)
		os.Exit(1)
	}
}

func StartCluster(s *State, cn string) {
	if exists, _ := s.Clusters.ClusterExists(cn); !exists {
		fmt.Printf("%s cluster does not exist\n", cn)
		os.Exit(1)
	}
	cmd := exec.Command(s.BinaryPaths["k3d"], "cluster", "start", cn)
	err := internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Printf("unable to start cluster: %s", err)
		os.Exit(1)
	}
}

func StopCluster(s *State, cn string) {
	if exists, _ := s.Clusters.ClusterExists(cn); !exists {
		fmt.Printf("%s cluster does not exist\n", cn)
		os.Exit(1)
	}
	cmd := exec.Command(s.BinaryPaths["k3d"], "cluster", "stop", cn)
	err := internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Printf("unable to stop cluster: %s", err)
		os.Exit(1)
	}
}

func DeleteCluster(s *State, cn string) {
	StopCluster(s, cn)
	cmd := exec.Command(s.BinaryPaths["k3d"], "cluster", "delete", cn)
	err := internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Printf("unable to delete cluster: %s", err)
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
	err := internal.RunCmdWithProgress(KubeCtl(s, "version"))
	if err != nil {
		fmt.Printf("could not get cluster version: %s\n", err)
	}
}

func ConfigureKeycloak(s *State) {
	// Configure keycloak.
	err := KubeExec(s, "lagoon-core", "lagoon-core-keycloak", `
set -ex
/opt/jboss/keycloak/bin/kcadm.sh config credentials \
  --server http://localhost:8080/auth --realm master \
  --user $KEYCLOAK_ADMIN_USER --password $KEYCLOAK_ADMIN_PASSWORD \
  --config /tmp/kcadm.config

/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon \
  -s resetPasswordAllowed=true --config /tmp/kcadm.config

/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon \
  -s smtpServer.host="mailhog.default.svc.cluster.local" --config /tmp/kcadm.config

/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon \
  -s smtpServer.port=1025 --config /tmp/kcadm.config

/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon \
  -s smtpServer.from="lagoon@k3d-rockpool" --config /tmp/kcadm.config

rm /tmp/kcadm.config
`,
	)
	if err != nil {
		fmt.Println("error configuring keycloak: ", internal.GetCmdStdErr(err))
	}
}

func (c *Config) ToMap() map[string]string {
	return map[string]string{
		"ClusterName":           c.ClusterName,
		"LagoonBaseUrl":         c.LagoonBaseUrl,
		"HarborPass":            c.HarborPass,
		"Arch":                  c.Arch,
		"RenderedTemplatesPath": c.RenderedTemplatesPath,
	}
}
