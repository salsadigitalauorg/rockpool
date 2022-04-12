package rockpool

import (
	"fmt"
	"os"
	"os/exec"
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
	r.DeleteCluster(r.ControllerClusterName())
	r.DeleteCluster(r.Config.ClusterName + "-target-1")
	r.wg.Wait()
}

func (r *Rockpool) LagoonController() {
	r.CreateCluster(r.ControllerClusterName())
	fmt.Println()

	r.GetClusterKubeConfigPath(r.ControllerClusterName())
	r.InstallMailHog()

	r.ClusterVersion(r.ControllerClusterName())
	fmt.Println()

	r.HelmList(r.ControllerClusterName())
	r.InstallIngressNginx()

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
	r.ClusterVersion(tgtCn)
	fmt.Println()

	r.HelmList(r.ControllerClusterName())
	r.InstallLagoonRemote(tgtCn)
}

func (r *Rockpool) InstallMailHog() {
	r.KubeApply(r.ControllerClusterName(), "default", "mailhog.yml.tmpl", true)
}

func (r *Rockpool) ConfigureKeycloak() {
	// Configure keycloak.
	err := r.KubeExec(
		r.ControllerClusterName(), "lagoon-core", "lagoon-core-keycloak", `
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
		os.Exit(1)
	}
}
