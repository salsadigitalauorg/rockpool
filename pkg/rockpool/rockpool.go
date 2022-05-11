package rockpool

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/yusufhm/rockpool/internal"
)

//go:embed templates
var templates embed.FS

func (c *Config) ToMap() map[string]string {
	return map[string]string{
		"Name":     c.Name,
		"Hostname": c.Hostname,
		"Arch":     c.Arch,
	}
}

// RenderTemplate executes a given template file and returns the path to its
// rendered version.
func (r *Rockpool) RenderTemplate(tn string, config interface{}, destName string) (string, error) {
	t := template.Must(template.ParseFS(templates, "templates/"+tn))

	var rendered string
	path := r.RenderedTemplatesPath()
	if destName != "" {
		rendered = filepath.Join(path, destName)
	} else if filepath.Ext(tn) == ".tmpl" {
		rendered = filepath.Join(path, strings.TrimSuffix(tn, ".tmpl"))
	} else {
		rendered = filepath.Join(path, tn)
	}

	f, err := os.Create(rendered)
	if err != nil {
		return "", err
	}

	err = t.Execute(f, config)
	f.Close()
	if err != nil {
		return "", err
	}
	return rendered, nil
}

func (r *Rockpool) Up(clusters []string) {
	if len(clusters) == 0 {
		if len(r.State.Clusters) > 0 {
			clusters = r.allClusters()
		} else {
			clusters = append(clusters, r.ControllerClusterName())
			for i := 1; i <= r.Config.NumTargets; i++ {
				clusters = append(clusters, r.TargetClusterName(i))
			}
		}
	}
	r.CreateRegistry()
	r.CreateClusters(clusters)
	r.InstallResolver()

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

	r.GetLagoonApiClient()
	r.LagoonApiGetRemotes()
	if len(setupTargets) > 0 {
		r.FetchHarborCerts()
		for _, c := range setupTargets {
			r.WgAdd(1)
			go r.SetupLagoonTarget(c)
		}
		r.WgWait()

		// Do the following serially so as not to run into
		// race conditions while doing the restarts.
		for _, c := range setupTargets {
			r.AddHarborHostEntries(c)
			r.InstallHarborCerts(c)
		}
	}
	fmt.Println()
	r.Status()
}

func (r *Rockpool) allClusters() []string {
	cls := []string{}
	for _, c := range r.State.Clusters {
		cls = append(cls, c.Name)
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
		if c == r.ControllerClusterName() {
			r.LagoonCliDeleteConfig()
			r.RemoveResolver()
		}
		r.WgAdd(1)
		go r.DeleteCluster(c)
	}
	r.WgWait()
}

func (r *Rockpool) CreateClusters(clusters []string) {
	r.FetchClusters()
	for _, c := range clusters {
		r.CreateCluster(c)
		r.WriteKubeConfig(c)
	}
	r.FetchClusters()
}

func (r *Rockpool) SetupLagoonController() {
	r.InstallMailHog()

	r.HelmList(r.ControllerClusterName())
	r.InstallIngressNginx()
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

	r.HelmList(cn)
	r.ConfigureTargetCoreDNS(cn)
	r.InstallNfsProvisioner(cn)
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
		fmt.Printf("[%s] unable to add gitea repo: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	values, err := r.RenderTemplate("gitea-values.yml.tmpl", r.Config, "")
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

func (r *Rockpool) InstallNfsProvisioner(cn string) {
	cmd := r.Helm(cn, "", "repo", "add", "nfs-provisioner", "https://kubernetes-sigs.github.io/nfs-ganesha-server-and-external-provisioner/")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("[%s] unable to add nfs-provisioner repo: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	values, err := r.RenderTemplate("nfs-server-provisioner-values.yml.tmpl", r.Config, "")
	if err != nil {
		fmt.Printf("[%s] error rendering nfs-provisioner values template: %s\n", cn, err)
		os.Exit(1)
	}
	fmt.Printf("[%s] using generated nfs-provisioner values at %s\n", cn, values)

	_, err = r.HelmInstallOrUpgrade(cn, "nfs-provisioner", "nfs", "nfs-provisioner/nfs-server-provisioner",
		[]string{"--create-namespace", "--wait", "-f", values},
	)
	if err != nil {
		fmt.Printf("[%s] unable to install nfs-provisioner: %s\n", cn, internal.GetCmdStdErr(err))
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
	dest := filepath.Join("/etc/resolver", r.Hostname)
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
	dest := filepath.Join("/etc/resolver", r.Hostname)
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
	gitea_entry := fmt.Sprintf("%s %s.%s\n", r.ControllerIP(), "gitea", r.Hostname)
	if !strings.Contains(corednsCm.Data.NodeHosts, gitea_entry) {
		corednsCm.Data.NodeHosts += gitea_entry
	}
	for _, h := range []string{"harbor", "broker", "ssh", "api"} {
		entry := fmt.Sprintf("%s %s.lagoon.%s\n", r.ControllerIP(), h, r.Hostname)
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
	graphql := fmt.Sprintf("http://api.lagoon.%s/graphql", r.Hostname)
	ui := fmt.Sprintf("http://ui.lagoon.%s", r.Hostname)

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
	cmd := exec.Command("lagoon", "config", "delete", "--lagoon", "rockpool", "--force")
	_, err := cmd.Output()
	if err != nil {
		panic(err)
	}
}
