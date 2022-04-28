package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/yusufhm/rockpool/internal"
)

func (c *Config) ToMap() map[string]string {
	return map[string]string{
		"ClusterName":           c.ClusterName,
		"LagoonBaseUrl":         c.LagoonBaseUrl,
		"HarborPass":            c.HarborPass,
		"Arch":                  c.Arch,
		"RenderedTemplatesPath": c.RenderedTemplatesPath,
	}
}

func (r *Rockpool) VerifyReqs() {
	binaries := []string{"k3d", "docker", "kubectl", "helm", "lagoon"}
	missing := []string{}
	r.State.BinaryPaths = map[string]string{}
	for _, b := range binaries {
		path, err := exec.LookPath(b)
		if err != nil {
			missing = append(missing, fmt.Sprintf("could not find %s; please ensure it is installed before", b))
			continue
		}
		r.State.BinaryPaths[b] = path
	}
	for _, m := range missing {
		fmt.Println(m)
	}
	if len(missing) > 0 {
		fmt.Println("some requirements were not met; please review above")
		os.Exit(1)
	}

	// Create temporary directory for rendered templates.
	err := os.MkdirAll(r.Config.RenderedTemplatesPath, os.ModePerm)
	if err != nil {
		fmt.Printf("unabled to create temp dir %s: %s\n", r.Config.RenderedTemplatesPath, err)
		os.Exit(1)
	}
}

func (r *Rockpool) Stop() {
	r.wg = &sync.WaitGroup{}
	r.wg.Add(2)
	go r.StopCluster(r.ControllerClusterName())
	go r.StopCluster(r.Config.ClusterName + "-target-1")
	r.wg.Wait()
}

func (r *Rockpool) Down() {
	r.wg = &sync.WaitGroup{}
	r.wg.Add(2)
	go r.DeleteCluster(r.ControllerClusterName())
	go r.DeleteCluster(r.Config.ClusterName + "-target-1")
	r.wg.Wait()
}

func (r *Rockpool) LagoonController() {
	r.CreateCluster(r.ControllerClusterName())
	r.DockerControllerIP()
	fmt.Println()

	r.GetClusterKubeConfigPath(r.ControllerClusterName())
	r.ClusterVersion(r.ControllerClusterName())
	fmt.Println()

	r.InstallMailHog()

	r.HelmList(r.ControllerClusterName())
	r.InstallIngressNginx()

	// r.InstallGitlab()
	r.InstallGitea()

	r.InstallHarbor()
	r.InstallLagoonCore()

	// Wait for Keycloak to be installed, then configure it.
	r.ConfigureKeycloak()
}

func (r *Rockpool) ControllerClusterName() string {
	return r.Config.ClusterName + "-controller"
}

func (r *Rockpool) LagoonTarget() {
	tgtCn := r.Config.ClusterName + "-target-1"
	r.CreateCluster(tgtCn)
	fmt.Println()

	r.GetClusterKubeConfigPath(tgtCn)
	r.ConfigureTargetCoreDNS(tgtCn)
	r.ClusterVersion(tgtCn)
	fmt.Println()

	r.HelmList(tgtCn)
	r.InstallLagoonRemote(tgtCn)
}

func (r *Rockpool) InstallMailHog() {
	_, err := r.KubeApply(r.ControllerClusterName(), "default", "mailhog.yml.tmpl", true)
	if err != nil {
		fmt.Println("unable to install mailhog: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallGitlab() {
	_, err := r.KubeApply(r.ControllerClusterName(), "gitlab", "gitlab.yml.tmpl", true)
	if err != nil {
		fmt.Println("unable to install gitlab: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallGitea() {
	_, err := r.KubeApply(r.ControllerClusterName(), "gitea", "gitea.yml.tmpl", true)
	if err != nil {
		fmt.Println("unable to install gitea: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) ConfigureKeycloak() {
	if _, err := r.KubeExecNoProgress(
		r.ControllerClusterName(), "lagoon-core", "lagoon-core-keycloak", `
set -e
rm -f /tmp/kcadm.config
/opt/jboss/keycloak/bin/kcadm.sh config credentials \
  --server http://localhost:8080/auth --realm master \
  --user $KEYCLOAK_ADMIN_USER --password $KEYCLOAK_ADMIN_PASSWORD \
  --config /tmp/kcadm.config
`,
	).Output(); err != nil {
		fmt.Println("error logging in to Keycloak: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	// Skip if values have already been set.
	if out, err := r.KubeExecNoProgress(
		r.ControllerClusterName(), "lagoon-core", "lagoon-core-keycloak", `
set -e
/opt/jboss/keycloak/bin/kcadm.sh get realms/lagoon \
	--fields 'smtpServer(from)' --config /tmp/kcadm.config
`,
	).Output(); err != nil {
		fmt.Println("error checking keycloak configuration: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	} else {
		s := struct {
			SmtpServer struct {
				From string `json:"from"`
			} `json:"smtpServer"`
		}{}
		err := json.Unmarshal(out, &s)
		if err != nil {
			fmt.Println("error parsing keycloak configuration: ", err)
			os.Exit(1)
		}
		if s.SmtpServer.From == "lagoon@k3d-rockpool" {
			fmt.Println("keycloak already configured")
			return
		}
	}

	// Configure keycloak.
	_, err := r.KubeExecNoProgress(
		r.ControllerClusterName(), "lagoon-core", "lagoon-core-keycloak", `
set -e

/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon \
  -s resetPasswordAllowed=true --config /tmp/kcadm.config

/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon \
  -s smtpServer.host="mailhog.default.svc.cluster.local" --config /tmp/kcadm.config

/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon \
  -s smtpServer.port=1025 --config /tmp/kcadm.config

/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon \
  -s smtpServer.from="lagoon@k3d-rockpool" --config /tmp/kcadm.config
`,
	).Output()
	if err != nil {
		fmt.Println("error configuring keycloak: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

// ConfigureTargetCoreDNS adds DNS records to targets for the required services.
func (r *Rockpool) ConfigureTargetCoreDNS(cn string) {
	cm := r.KubeGetConfigMap(cn, "kube-system", "coredns")
	corednsCm := CoreDNSConfigMap{}
	err := json.Unmarshal(cm, &corednsCm)
	if err != nil {
		fmt.Println("error parsing CoreDNS configmap: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	gitlab_entry := fmt.Sprintf("%s %s.%s\n", r.ControllerDockerIP, "gitlab", r.Hostname)
	if strings.Contains(corednsCm.Data.NodeHosts, gitlab_entry) {
		fmt.Println("CoreDNS already contains the records")
		return
	}
	corednsCm.Data.NodeHosts += gitlab_entry
	for _, h := range []string{"harbor", "broker", "ssh", "api"} {
		entry := fmt.Sprintf("%s %s.%s\n", r.ControllerDockerIP, h, r.LagoonBaseUrl)
		corednsCm.Data.NodeHosts += entry
	}
	cm, err = json.Marshal(corednsCm)
	if err != nil {
		fmt.Println("error encoding CoreDNS configmap: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	r.KubeReplace(cn, "kube-system", "coredns", string(cm))
}
