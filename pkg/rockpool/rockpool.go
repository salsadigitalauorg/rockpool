package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
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

func (r *Rockpool) Up() {
	r.VerifyReqs(true)
	r.FetchClusters()
	r.CreateRegistry()
	r.CreateClusters()
	r.LagoonController()
	r.LagoonTarget()
	r.InstallHarborCerts()
}

func (r *Rockpool) Start() {
	r.wg = &sync.WaitGroup{}
	r.wg.Add(2)
	go r.StartCluster(r.ControllerClusterName())
	go r.StartCluster(r.TargetClusterName(1))
	r.wg.Wait()
	r.wg = nil
}

func (r *Rockpool) Stop() {
	r.wg = &sync.WaitGroup{}
	r.wg.Add(2)
	go r.StopCluster(r.ControllerClusterName())
	go r.StopCluster(r.TargetClusterName(1))
	r.wg.Wait()
	r.wg = nil
}

func (r *Rockpool) Down() {
	r.wg = &sync.WaitGroup{}
	r.wg.Add(2)
	go r.DeleteCluster(r.ControllerClusterName())
	go r.DeleteCluster(r.TargetClusterName(1))
	r.wg.Wait()
	r.wg = nil
}

func (r *Rockpool) CreateClusters() {
	r.FetchClusters()
	r.wg = &sync.WaitGroup{}
	r.wg.Add(2)
	go r.CreateCluster(r.ControllerClusterName())
	go r.CreateCluster(r.TargetClusterName(1))
	r.wg.Wait()
	r.wg = nil
	r.FetchClusters()
}

func (r *Rockpool) LagoonController() {
	r.GetClusterKubeConfigPath(r.ControllerClusterName())

	r.InstallMailHog()

	r.HelmList(r.ControllerClusterName())
	r.InstallIngressNginx()
	r.InstallCertManager()

	// r.InstallGitlab()
	r.InstallGitea()

	r.InstallHarbor()
	r.InstallLagoonCore()

	// Wait for Keycloak to be installed, then configure it.
	r.ConfigureKeycloak()
}

func (r *Rockpool) LagoonTarget() {
	tgtCn := r.TargetClusterName(1)
	r.CreateCluster(tgtCn)

	r.GetClusterKubeConfigPath(tgtCn)
	r.ConfigureTargetCoreDNS(tgtCn)

	r.HelmList(tgtCn)
	r.InstallLagoonRemote(tgtCn)
}

func (r *Rockpool) InstallMailHog() {
	_, err := r.KubeApplyTemplate(r.ControllerClusterName(), "default", "mailhog.yml.tmpl", true)
	if err != nil {
		fmt.Println("unable to install mailhog: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallCertManager() {
	_, err := r.KubeApplyTemplate(r.ControllerClusterName(), "", "cert-manager.yaml", true)
	if err != nil {
		fmt.Println("unable to install cert-manager: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	_, err = r.KubeApplyTemplate(r.ControllerClusterName(), "cert-manager", "ca.yml.tmpl", true)
	if err != nil {
		fmt.Println("unable to install cert-manager: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallGitlab() {
	_, err := r.KubeApplyTemplate(r.ControllerClusterName(), "gitlab", "gitlab.yml.tmpl", true)
	if err != nil {
		fmt.Println("unable to install gitlab: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallGitea() {
	_, err := r.KubeApplyTemplate(r.ControllerClusterName(), "gitea", "gitea.yml.tmpl", true)
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
	gitea_entry := fmt.Sprintf("%s %s.%s\n", r.ControllerIP(), "gitea", r.Hostname)
	if !strings.Contains(corednsCm.Data.NodeHosts, gitea_entry) {
		corednsCm.Data.NodeHosts += gitea_entry
	}
	for _, h := range []string{"harbor", "broker", "ssh", "api"} {
		entry := fmt.Sprintf("%s %s.%s\n", r.ControllerIP(), h, r.LagoonBaseUrl)
		if !strings.Contains(corednsCm.Data.NodeHosts, entry) {
			corednsCm.Data.NodeHosts += entry
		}
	}
	cm, err = json.Marshal(corednsCm)
	if err != nil {
		fmt.Println("error encoding CoreDNS configmap: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	r.KubeReplace(cn, "kube-system", "coredns", string(cm))
	r.KubeCtl(cn, "kube-system", "rollout", "restart", "deployment/coredns")
}
