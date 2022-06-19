package rockpool

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/salsadigitalauorg/rockpool/internal"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
)

//go:embed templates
var templates embed.FS

func EnsureBinariesExist() {
	binaries := []string{"k3d", "docker", "kubectl", "helm", "lagoon"}
	missing := []string{}
	for _, b := range binaries {
		_, err := exec.LookPath(b)
		if err != nil {
			missing = append(missing, fmt.Sprintf("[rockpool] could not find %s; please ensure it is installed and can be found in the $PATH", b))
			continue
		}
	}
	for _, m := range missing {
		fmt.Println(m)
	}
	if len(missing) > 0 {
		fmt.Println("[rockpool] some requirements were not met; please review above")
		os.Exit(1)
	}
}

func (c *Config) ToMap() map[string]string {
	return map[string]string{
		"Name":     c.Name,
		"Domain":   c.Domain,
		"Hostname": fmt.Sprintf("%s.%s", c.Name, c.Domain),
		"Arch":     c.Arch,
	}
}

func (r *Rockpool) Initialise() {
	EnsureBinariesExist()
	ts := Templates{Config: &r.Config}
	r.Templates = &ts

	d := Docker{}
	r.Docker = &d

	k3 := K3d{
		PlatformName: r.Name,
		Docker:       r.Docker,
		Templates:    r.Templates,
		Wg:           &r.Wg,
	}
	r.K3d = &k3

	r.Spinner.Color("red", "bold")
	r.Config.Arch = runtime.GOARCH

	// Create directory for rendered templates.
	err := os.MkdirAll(r.Templates.RenderedPath(true), os.ModePerm)
	if err != nil {
		fmt.Printf("[rockpool] unable to create temp dir %s: %s\n", r.Templates.RenderedPath(true), err)
		os.Exit(1)
	}

	r.ClusterFetch()
}

func (r *Rockpool) Up(clusters []string) {
	if len(clusters) == 0 {
		if len(r.K3d.Clusters) > 0 {
			clusters = r.allClusters()
		} else {
			clusters = append(clusters, r.ControllerClusterName())
			for i := 1; i <= r.Config.NumTargets; i++ {
				clusters = append(clusters, r.TargetClusterName(i))
			}
		}
	}
	r.K3d.RegistryCreate()
	r.K3d.RegistryRenderConfig()
	r.CreateClusters(clusters)

	setupController := false
	setupTargets := []string{}
	for _, c := range clusters {
		if c == r.ControllerClusterName() {
			setupController = true
			continue
		}
		setupTargets = append(setupTargets, c)
	}

	if setupController {
		r.SetupLagoonController()
	}

	r.GetLagoonApiClient()
	r.LagoonApiGetRemotes()
	if len(setupTargets) > 0 {
		r.FetchHarborCerts()
		for _, c := range setupTargets {
			r.WgAdd(1)
			go r.SetupLagoonTarget(c)
		}
		r.WgWait()

		r.SetupNginxReverseProxyForRemotes()

		// Do the following serially so as not to run into
		// race conditions while doing the restarts.
		for _, c := range setupTargets {
			r.AddHarborHostEntries(c)
			r.InstallHarborCerts(c)
		}
	}
	r.InstallResolver()
	fmt.Println()
	r.Status()
}

func (r *Rockpool) allClusters() []string {
	cls := []string{}
	for _, c := range r.K3d.Clusters {
		cls = append(cls, c.Name)
	}
	return cls
}

func (r *Rockpool) Start(clusters []string) {
	r.K3d.RegistryStart()
	if len(clusters) == 0 {
		clusters = r.allClusters()
	}
	for _, cn := range clusters {
		r.K3d.ClusterStart(cn)
		r.AddHarborHostEntries(cn)
		if cn != r.ControllerClusterName() {
			r.ConfigureTargetCoreDNS(cn)
		}
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
			r.ClusterStop(c)
		}(c)
	}
	r.WgWait()
	r.K3d.RegistryStop()
}

func (r *Rockpool) Down(clusters []string) {
	if len(clusters) == 0 {
		clusters = r.allClusters()
	}
	for _, c := range clusters {
		if c == r.ControllerClusterName() {
			r.LagoonCliDeleteConfig()
			r.RemoveResolver()
		}
		r.WgAdd(1)
		go r.ClusterDelete(c)
	}
	r.WgWait()
	r.K3d.RegistryStop()
}

func (r *Rockpool) CreateClusters(clusters []string) {
	for _, c := range clusters {
		r.ClusterCreate(c, c == r.ControllerClusterName())
		r.WriteKubeConfig(c)
	}
}

func (r *Rockpool) SetupLagoonController() {
	r.InstallMailHog()

	helm.List(r.ControllerClusterName())
	r.InstallIngressNginx(r.ControllerClusterName())
	r.InstallCertManager()

	r.InstallDnsmasq()

	// r.InstallGitlab()
	r.InstallGitea()

	// Create test repo.
	r.GiteaCreateRepo()

	r.InstallHarbor()
	r.InstallLagoonCore()

	// Wait for Keycloak to be installed, then configure it.
	r.ConfigureKeycloak()
	r.GetLagoonApiClient()
	r.LagoonApiAddSshKey()
	r.LagoonCliAddConfig()
}

func (r *Rockpool) SetupLagoonTarget(cn string) {
	defer r.WgDone()

	helm.List(cn)
	r.ConfigureTargetCoreDNS(cn)
	r.InstallIngressNginx(cn)
	r.InstallNfsProvisioner(cn)
	r.InstallMariaDB(cn)
	r.InstallLagoonRemote(cn)
	r.RegisterLagoonRemote(cn)
}

func (r *Rockpool) InstallMailHog() {
	cn := r.ControllerClusterName()
	_, err := r.KubeApplyTemplate(cn, "default", "mailhog.yml.tmpl", true)
	if err != nil {
		fmt.Printf("[%s] unable to install mailhog: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) SetupNginxReverseProxyForRemotes() {
	cn := r.ControllerClusterName()

	cm := map[string]interface{}{
		"Name":   r.Config.Name,
		"Domain": r.Config.Domain,
	}
	targets := map[int]string{}
	for i := 0; i < r.Config.NumTargets; i++ {
		targets[i+1] = r.TargetIP(r.TargetClusterName(i + 1))
	}
	cm["Targets"] = targets

	patchFile, err := r.Templates.Render("ingress-nginx-values.yml.tmpl", cm, "")
	if err != nil {
		fmt.Printf("[%s] error rendering ingress nginx patch template: %s\n", cn, err)
		os.Exit(1)
	}

	fmt.Printf("[%s] using generated manifest at %s\n", cn, patchFile)
	_, err = r.KubeApply(cn, "ingress-nginx", patchFile, true)
	if err != nil {
		fmt.Printf("[%s] unable to setup nginx reverse proxy: %s\n", cn, internal.GetCmdStdErr(err))
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
	cmd := helm.Exec(cn, "", "repo", "add", "gitea-charts", "https://dl.gitea.io/charts/")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("[%s] unable to add gitea repo: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	values, err := r.Templates.Render("gitea-values.yml.tmpl", r.Config.ToMap(), "")
	if err != nil {
		fmt.Printf("[%s] error rendering gitea values template: %s\n", cn, err)
		os.Exit(1)
	}
	fmt.Printf("[%s] using generated gitea values at %s\n", cn, values)

	_, err = helm.InstallOrUpgrade(cn, "gitea", "gitea", "gitea-charts/gitea",
		[]string{"--create-namespace", "--wait", "-f", values},
	)
	if err != nil {
		fmt.Printf("[%s] unable to install gitea: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallNfsProvisioner(cn string) {
	cmd := helm.Exec(cn, "", "repo", "add", "nfs-provisioner", "https://kubernetes-sigs.github.io/nfs-ganesha-server-and-external-provisioner/")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("[%s] unable to add nfs-provisioner repo: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	values, err := r.Templates.Render("nfs-server-provisioner-values.yml.tmpl", r.Config.ToMap(), "")
	if err != nil {
		fmt.Printf("[%s] error rendering nfs-provisioner values template: %s\n", cn, err)
		os.Exit(1)
	}
	fmt.Printf("[%s] using generated nfs-provisioner values at %s\n", cn, values)

	_, err = helm.InstallOrUpgrade(cn, "nfs-provisioner", "nfs", "nfs-provisioner/nfs-server-provisioner",
		[]string{"--create-namespace", "--wait", "-f", values},
	)
	if err != nil {
		fmt.Printf("[%s] unable to install nfs-provisioner: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallMariaDB(cn string) {
	cmd := helm.Exec(cn, "", "repo", "add", "nicholaswilde", "https://nicholaswilde.github.io/helm-charts/")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("[%s] unable to add nicholaswilde repo: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	_, err = r.KubeApply(cn, "", "https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/mariadb.yaml", true)
	if err != nil {
		fmt.Printf("[%s] unable to install mariadb crds: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	_, err = r.KubeApply(cn, "", "https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/mongodb.yaml", true)
	if err != nil {
		fmt.Printf("[%s] unable to install mongodb crds: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	_, err = r.KubeApply(cn, "", "https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/postgres.yaml", true)
	if err != nil {
		fmt.Printf("[%s] unable to install postgres crds: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	_, err = helm.InstallOrUpgrade(cn, "mariadb", "mariadb-production", "nicholaswilde/mariadb",
		[]string{
			"--create-namespace", "--wait",
			"--set", "fullnameOverride=production",
			"--set", "secret.MYSQL_ROOT_PASSWORD=mariadbpass",
			"--set", "persistence.config.enabled=true",
		},
	)
	if err != nil {
		fmt.Printf("[%s] unable to install mariadb-production: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallDnsmasq() {
	cn := r.ControllerClusterName()
	_, err := r.KubeApplyTemplate(cn, "default", "dnsmasq.yml.tmpl", true)
	if err != nil {
		fmt.Printf("[%s] unable to install dnsmasq: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallResolver() {
	dest := filepath.Join("/etc/resolver", r.Hostname())
	data := `
nameserver 127.0.0.1
port 6153
`

	var tmpFile *os.File
	var err error

	if _, err := os.Stat(dest); err == nil {
		fmt.Println("[rockpool] resolver file already exists")
		return
	}

	fmt.Println("[rockpool] creating resolver file")
	if tmpFile, err = ioutil.TempFile("", "rockpool-resolver-"); err != nil {
		panic(err)
	}
	if err = os.Chmod(tmpFile.Name(), 0777); err != nil {
		panic(err)
	}
	if _, err = tmpFile.WriteString(data); err != nil {
		panic(err)
	}
	if _, err = exec.Command("sudo", "mv", tmpFile.Name(), dest).Output(); err != nil {
		panic(internal.GetCmdStdErr(err))
	}
}

func (r *Rockpool) RemoveResolver() {
	dest := filepath.Join("/etc/resolver", r.Hostname())
	if _, err := exec.Command("rm", "-f", dest).Output(); err != nil {
		fmt.Println(err)
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
	for _, h := range []string{"harbor", "broker", "ssh", "api", "gitea"} {
		entry := fmt.Sprintf("%s %s.lagoon.%s\n", r.ControllerIP(), h, r.Hostname())
		if !strings.Contains(corednsCm.Data.NodeHosts, entry) {
			corednsCm.Data.NodeHosts += entry
		}
	}

	cm, err = json.Marshal(corednsCm)
	if err != nil {
		fmt.Printf("[%s] error encoding CoreDNS configmap: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	fmt.Printf("[%s] %s", cn, r.KubeReplace(cn, "kube-system", "coredns", string(cm)))
	out, err := r.KubeCtl(cn, "kube-system", "rollout", "restart", "deployment/coredns").Output()
	if err != nil {
		fmt.Printf("[%s] CoreDNS restart failed: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	fmt.Printf("[%s] %s", cn, string(out))
}

func (r *Rockpool) LagoonCliAddConfig() {
	graphql := fmt.Sprintf("http://api.lagoon.%s/graphql", r.Hostname())
	ui := fmt.Sprintf("http://ui.lagoon.%s", r.Hostname())

	// Get list of existing configs.
	cmd := exec.Command("lagoon", "config", "list", "--output-json")
	out, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	var configs struct {
		Data []struct {
			GraphQl     string `json:"graphql"`
			Ui          string `json:"ui-url"`
			SshHostname string `json:"ssh-hostname"`
		}
	}
	err = json.Unmarshal(out, &configs)
	if err != nil {
		panic(err)
	}

	// Add the config.
	fmt.Println("[rockpool] adding lagoon config")
	cmd = exec.Command("lagoon", "config", "add", "--lagoon", r.Name,
		"--graphql", graphql, "--ui", ui, "--hostname", "127.0.0.1", "--port", "2022")
	_, err = cmd.Output()
	if err != nil {
		panic(err)
	}
}

func (r *Rockpool) LagoonCliDeleteConfig() {
	// Get list of existing configs.
	cmd := exec.Command("lagoon", "config", "delete", "--lagoon", r.Name, "--force")
	_, err := cmd.Output()
	if err != nil {
		panic(err)
	}
}
