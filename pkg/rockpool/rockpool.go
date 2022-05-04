package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

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

func (r *Rockpool) Up(clusters []string) {
	if len(clusters) == 0 {
		clusters = r.allClusters()
	}
	r.VerifyReqs(true)
	r.FetchClusters()
	r.CreateRegistry()
	r.CreateClusters(clusters)

	setupController := false
	setupTargets := []string{}
	for _, c := range clusters {
		if c == "rockpool-controller" {
			setupController = true
			continue
		}
		setupTargets = append(setupTargets, c)
	}

	if setupController {
		r.SetupLagoonController()
	}
	r.State.KeycloakUrl = fmt.Sprintf("http://keycloak.%s/auth", r.Config.LagoonBaseUrl)

	r.GetLagoonApiClient()
	r.LagoonApiGetRemotes()
	if len(setupTargets) > 0 {
		for _, c := range setupTargets {
			r.WgAdd(1)
			go r.SetupLagoonTarget(c)
		}
		r.WgWait()
	}
}

func (r *Rockpool) allClusters() []string {
	cls := []string{r.ControllerClusterName()}
	for i := 1; i <= r.Config.NumTargets; i++ {
		cls = append(cls, r.TargetClusterName(i))
	}
	return cls
}

func (r *Rockpool) Start(clusters []string) {
	if len(clusters) == 0 {
		clusters = r.allClusters()
	}
	for _, c := range clusters {
		r.StartCluster(c)
	}
}

func (r *Rockpool) Stop(clusters []string) {
	if len(clusters) == 0 {
		clusters = r.allClusters()
	}
	for _, c := range clusters {
		r.WgAdd(1)
		go func(c string) {
			defer r.WgDone()
			r.StopCluster(c)
		}(c)
	}
	r.WgWait()
}

func (r *Rockpool) Down(clusters []string) {
	if len(clusters) == 0 {
		clusters = r.allClusters()
	}
	for _, c := range clusters {
		r.WgAdd(1)
		go r.DeleteCluster(c)
	}
	r.WgWait()
}

func (r *Rockpool) CreateClusters(clusters []string) {
	r.FetchClusters()
	for _, c := range clusters {
		r.CreateCluster(c)
	}
	r.FetchClusters()
}

func (r *Rockpool) SetupLagoonController() {
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

func (r *Rockpool) SetupLagoonTarget(cn string) {
	defer r.WgDone()
	r.GetClusterKubeConfigPath(cn)

	r.HelmList(cn)
	r.ConfigureTargetCoreDNS(cn)
	r.InstallLagoonRemote(cn)
	r.RegisterLagoonRemote(cn)

	r.InstallHarborCerts(cn)
}

func (r *Rockpool) InstallMailHog() {
	cn := r.ControllerClusterName()
	_, err := r.KubeApplyTemplate(cn, "default", "mailhog.yml.tmpl", true)
	if err != nil {
		fmt.Printf("[%s] unable to install mailhog: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallCertManager() {
	cn := r.ControllerClusterName()
	_, err := r.KubeApplyTemplate(cn, "", "cert-manager.yaml", true)
	if err != nil {
		fmt.Printf("[%s] unable to install cert-manager: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	retries := 10
	deployNotFound := true
	var failedErr error
	for deployNotFound && retries > 0 {
		failedErr = nil
		_, err = r.KubeCtl(r.ControllerClusterName(), "cert-manager",
			"wait", "--for=condition=Available=true", "deployment/cert-manager-webhook").Output()
		if err != nil {
			failedErr = err
			retries--
			time.Sleep(5 * time.Second)
			continue
		}
		deployNotFound = false
	}
	if failedErr != nil {
		fmt.Printf("[%s] error while waiting for cert-manager webhook: %s\n", cn, internal.GetCmdStdErr(failedErr))
		os.Exit(1)
	}

	retries = 30
	failed := true
	for retries > 0 && failed {
		failedErr = nil
		_, err = r.KubeApplyTemplate(r.ControllerClusterName(), "cert-manager", "ca.yml.tmpl", true)
		if err != nil {
			failed = true
			failedErr = err
			retries--
			time.Sleep(10 * time.Second)
			continue
		}
		failed = false
	}
	if failed {
		fmt.Printf("[%s] unable to install cert-manager: %s\n", cn, internal.GetCmdStdErr(failedErr))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallGitlab() {
	cn := r.ControllerClusterName()
	_, err := r.KubeApplyTemplate(cn, "gitlab", "gitlab.yml.tmpl", true)
	if err != nil {
		fmt.Printf("[%s] unable to install gitlab: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallGitea() {
	cn := r.ControllerClusterName()
	cmd := r.Helm(cn, "", "repo", "add", "gitea-charts", "https://dl.gitea.io/charts/")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("[%s] unable to add harbor repo: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	values, err := internal.RenderTemplate("gitea-values.yml.tmpl", r.Config.RenderedTemplatesPath, r.Config, "")
	if err != nil {
		fmt.Printf("[%s] error rendering gitea values template: %s\n", cn, err)
		os.Exit(1)
	}
	fmt.Printf("[%s] using generated gitea values at %s\n", cn, values)

	_, err = r.HelmInstallOrUpgrade(cn, "gitea", "gitea", "gitea-charts/gitea",
		[]string{"--create-namespace", "--wait", "-f", values},
	)
	if err != nil {
		fmt.Printf("[%s] unable to install gitea: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) ConfigureKeycloak() {
	cn := r.ControllerClusterName()
	if _, err := r.KubeExecNoProgress(
		cn, "lagoon-core", "lagoon-core-keycloak", `
set -e
rm -f /tmp/kcadm.config
/opt/jboss/keycloak/bin/kcadm.sh config credentials \
  --server http://localhost:8080/auth --realm master \
  --user $KEYCLOAK_ADMIN_USER --password $KEYCLOAK_ADMIN_PASSWORD \
  --config /tmp/kcadm.config
`,
	).Output(); err != nil {
		fmt.Printf("[%s] error logging in to Keycloak: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	// Skip if values have already been set.
	if out, err := r.KubeExecNoProgress(
		cn, "lagoon-core", "lagoon-core-keycloak", `
set -e
/opt/jboss/keycloak/bin/kcadm.sh get realms/lagoon \
	--fields 'smtpServer(from)' --config /tmp/kcadm.config
`,
	).Output(); err != nil {
		fmt.Printf("[%s] error checking keycloak configuration: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	} else {
		s := struct {
			SmtpServer struct {
				From string `json:"from"`
			} `json:"smtpServer"`
		}{}
		err := json.Unmarshal(out, &s)
		if err != nil {
			fmt.Printf("[%s] error parsing keycloak configuration: %s\n", cn, err)
			os.Exit(1)
		}
		if s.SmtpServer.From == "lagoon@k3d-rockpool" {
			fmt.Printf("[%s] keycloak already configured\n", cn)
			return
		}
	}

	// Configure keycloak.
	_, err := r.KubeExecNoProgress(cn, "lagoon-core", "lagoon-core-keycloak", `
set -e

/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon \
  -s resetPasswordAllowed=true --config /tmp/kcadm.config

/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon \
  -s smtpServer.host="mailhog.default.svc.cluster.local" --config /tmp/kcadm.config

/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon \
  -s smtpServer.port=1025 --config /tmp/kcadm.config

/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon \
  -s smtpServer.from="lagoon@k3d-rockpool" --config /tmp/kcadm.config

# Allow direct access grants so we can grab a token by using a POST request.
client=$(/opt/jboss/keycloak/bin/kcadm.sh get realms/lagoon/clients \
	--fields 'id,clientId' --config /tmp/kcadm.config \
	--format csv|grep "lagoon-ui")
client_id=$(echo ${client%,*} | sed 's/"//g')
/opt/jboss/keycloak/bin/kcadm.sh update realms/lagoon/clients/${client_id} \
	-s directAccessGrantsEnabled=true --config /tmp/kcadm.config
`,
	).Output()
	if err != nil {
		fmt.Printf("[%s] error configuring keycloak: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

// ConfigureTargetCoreDNS adds DNS records to targets for the required services.
func (r *Rockpool) ConfigureTargetCoreDNS(cn string) {
	cm := r.KubeGetConfigMap(cn, "kube-system", "coredns")
	corednsCm := CoreDNSConfigMap{}
	err := json.Unmarshal(cm, &corednsCm)
	if err != nil {
		fmt.Printf("[%s] error parsing CoreDNS configmap: %s\n", cn, internal.GetCmdStdErr(err))
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
		fmt.Printf("[%s] error encoding CoreDNS configmap: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	fmt.Printf("[%s] %s\n", cn, r.KubeReplace(cn, "kube-system", "coredns", string(cm)))
	out, err := r.KubeCtl(cn, "kube-system", "rollout", "restart", "deployment/coredns").Output()
	if err != nil {
		fmt.Printf("[%s] CoreDNS restart failed: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	fmt.Printf("[%s] %s\n", cn, string(out))
}
