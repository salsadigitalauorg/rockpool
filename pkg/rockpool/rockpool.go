package rockpool

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/salsadigitalauorg/rockpool/internal"
	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/gitea"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
	"github.com/salsadigitalauorg/rockpool/pkg/k3d"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
	"github.com/salsadigitalauorg/rockpool/pkg/lagoon"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	"github.com/salsadigitalauorg/rockpool/pkg/platform/templates"

	"github.com/briandowns/spinner"
	log "github.com/sirupsen/logrus"
)

var Spinner spinner.Spinner

func EnsureBinariesExist() {
	binaries := []string{"k3d", "docker", "kubectl", "helm", "lagoon"}
	missing := []string{}
	versionError := false
	for _, b := range binaries {
		absPath, err := exec.LookPath(b)
		if err != nil {
			missing = append(missing, fmt.Sprintf("[rockpool] could not find %s; please ensure it is installed and can be found in the $PATH", b))
			continue
		}
		versionCmd := command.ShellCommander(absPath, "version")
		if b == "kubectl" {
			versionCmd.AddArgs("--client", "--short")
		}
		if b == "docker" {
			versionCmd.AddArgs("--format", "json")
		}
		out, err := versionCmd.Output()
		if err != nil {
			log.WithFields(log.Fields{
				"binary": b,
				"error":  command.GetMsgFromCommandError(err),
			}).Error("Error getting version")
			versionError = true
		}
		log.WithFields(log.Fields{
			"binary": b,
			"result": string(out),
		}).Debug("fetched version")
	}
	for _, m := range missing {
		log.WithField("binary", m).Error("missing binary")
	}
	if len(missing) > 0 || versionError {
		log.Fatal("[rockpool] some requirements were not met; please review above")
	}
}

func Initialise() {
	log.Info("checking if binaries exist")
	EnsureBinariesExist()

	Spinner = *spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	Spinner.Color("red", "bold")

	// Create directory for rendered templates.
	templDir := templates.RenderedPath(true)
	log.WithField("dir", templDir).Info("creating directory for rendered templates")
	err := os.MkdirAll(templDir, os.ModePerm)
	if err != nil {
		log.Fatal("[rockpool] unable to create temp dir %s: %s\n", templates.RenderedPath(true), err)
	}
}

func Up(clusters []string) {
	k3d.ClusterFetch()
	if len(clusters) == 0 {
		if len(k3d.Clusters) > 0 {
			clusters = allClusters()
		} else {
			clusters = append(clusters, platform.ControllerClusterName())
			for i := 1; i <= platform.NumTargets; i++ {
				clusters = append(clusters, platform.TargetClusterName(i))
			}
		}
	}
	k3d.RegistryCreate()
	k3d.RegistryRenderConfig()
	k3d.RegistryStart()
	CreateClusters(clusters)

	setupController := false
	setupTargets := []string{}
	for _, c := range clusters {
		if c == platform.ControllerClusterName() {
			setupController = true
			continue
		}
		setupTargets = append(setupTargets, c)
	}

	if setupController {
		SetupLagoonController()
	}

	lagoon.InitApiClient()
	lagoon.GetRemotes()
	if len(setupTargets) > 0 {
		FetchHarborCerts()
		for _, c := range setupTargets {
			platform.WgAdd(1)
			go SetupLagoonTarget(c)
		}
		platform.WgWait()

		SetupNginxReverseProxyForRemotes()

		// Do the following serially so as not to run into
		// race conditions while doing the restarts.
		for _, c := range setupTargets {
			AddHarborHostEntries(c)
			InstallHarborCerts(c)
		}
	}
	InstallResolver()
	fmt.Println()
	Status()
}

func allClusters() []string {
	cls := []string{}
	for _, c := range k3d.Clusters {
		cls = append(cls, c.Name)
	}
	return cls
}

func Start(clusters []string) {
	k3d.RegistryStart()
	if len(clusters) == 0 {
		clusters = allClusters()
	}
	for _, cn := range clusters {
		k3d.ClusterStart(cn)
		AddHarborHostEntries(cn)
		if cn != platform.ControllerClusterName() {
			ConfigureTargetCoreDNS(cn)
		}
	}
}

func Stop(clusters []string) {
	if len(clusters) == 0 {
		clusters = allClusters()
	}
	for _, c := range clusters {
		platform.WgAdd(1)
		go func(c string) {
			defer platform.WgDone()
			k3d.ClusterStop(c)
		}(c)
	}
	platform.WgWait()
	k3d.RegistryStop()
}

func Down(clusters []string) {
	if len(clusters) == 0 {
		clusters = allClusters()
	}
	for _, c := range clusters {
		if c == platform.ControllerClusterName() {
			LagoonCliDeleteConfig()
			RemoveResolver()
		}
		platform.WgAdd(1)
		go k3d.ClusterDelete(c)
	}
	platform.WgWait()
	k3d.RegistryStop()
}

func CreateClusters(clusters []string) {
	for _, c := range clusters {
		k3d.ClusterCreate(c, c == platform.ControllerClusterName())
		k3d.WriteKubeConfig(c)
	}
}

func SetupLagoonController() {
	InstallMailHog()

	helm.List(platform.ControllerClusterName())
	InstallIngressNginx(platform.ControllerClusterName())
	InstallCertManager()

	InstallDnsmasq()

	// InstallGitlab()
	InstallGitea()

	// Create test repo.
	gitea.CreateRepo()

	InstallHarbor()
	InstallLagoonCore()

	// Wait for Keycloak to be installed, then configure it.
	ConfigureKeycloak()
	lagoon.InitApiClient()
	lagoon.AddSshKey()
	LagoonCliAddConfig()
}

func SetupLagoonTarget(cn string) {
	defer platform.WgDone()

	helm.List(cn)
	ConfigureTargetCoreDNS(cn)
	InstallIngressNginx(cn)
	InstallNfsProvisioner(cn)
	InstallMariaDB(cn)
	InstallLagoonRemote(cn)
	RegisterLagoonRemote(cn)
}

func InstallMailHog() {
	cn := platform.ControllerClusterName()
	_, err := kube.ApplyTemplate(cn, "default", "mailhog.yml.tmpl", true)
	if err != nil {
		fmt.Printf("[%s] unable to install mailhog: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func SetupNginxReverseProxyForRemotes() {
	cn := platform.ControllerClusterName()

	cm := map[string]interface{}{
		"Name":   platform.Name,
		"Domain": platform.Domain,
	}
	targets := map[int]string{}
	for i := 0; i < platform.NumTargets; i++ {
		targets[i+1] = k3d.TargetIP(platform.TargetClusterName(i + 1))
	}
	cm["Targets"] = targets

	patchFile, err := templates.Render("ingress-nginx-values.yml.tmpl", cm, "")
	if err != nil {
		fmt.Printf("[%s] error rendering ingress nginx patch template: %s\n", cn, err)
		os.Exit(1)
	}

	fmt.Printf("[%s] using generated manifest at %s\n", cn, patchFile)
	_, err = kube.Apply(cn, "ingress-nginx", patchFile, true)
	if err != nil {
		fmt.Printf("[%s] unable to setup nginx reverse proxy: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func InstallCertManager() {
	cn := platform.ControllerClusterName()
	_, err := kube.ApplyTemplate(cn, "", "cert-manager.yaml", true)
	if err != nil {
		fmt.Printf("[%s] unable to install cert-manager: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	retries := 10
	deployNotFound := true
	var failedErr error
	for deployNotFound && retries > 0 {
		failedErr = nil
		_, err = kube.Cmd(platform.ControllerClusterName(), "cert-manager",
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
		_, err = kube.ApplyTemplate(platform.ControllerClusterName(), "cert-manager", "ca.yml.tmpl", true)
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

func InstallGitlab() {
	cn := platform.ControllerClusterName()
	_, err := kube.ApplyTemplate(cn, "gitlab", "gitlab.yml.tmpl", true)
	if err != nil {
		fmt.Printf("[%s] unable to install gitlab: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func InstallGitea() {
	cn := platform.ControllerClusterName()
	cmd := helm.Exec(cn, "", "repo", "add", "gitea-charts", "https://dl.gitea.io/charts/")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("[%s] unable to add gitea repo: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	values, err := templates.Render("gitea-values.yml.tmpl", platform.ToMap(), "")
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

func InstallNfsProvisioner(cn string) {
	cmd := helm.Exec(cn, "", "repo", "add", "nfs-provisioner", "https://kubernetes-sigs.github.io/nfs-ganesha-server-and-external-provisioner/")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("[%s] unable to add nfs-provisioner repo: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	values, err := templates.Render("nfs-server-provisioner-values.yml.tmpl", platform.ToMap(), "")
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

func InstallMariaDB(cn string) {
	cmd := helm.Exec(cn, "", "repo", "add", "nicholaswilde", "https://nicholaswilde.github.io/helm-charts/")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("[%s] unable to add nicholaswilde repo: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	_, err = kube.Apply(cn, "", "https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/mariadb.yaml", true)
	if err != nil {
		fmt.Printf("[%s] unable to install mariadb crds: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	_, err = kube.Apply(cn, "", "https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/mongodb.yaml", true)
	if err != nil {
		fmt.Printf("[%s] unable to install mongodb crds: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	_, err = kube.Apply(cn, "", "https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/postgres.yaml", true)
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

	_, err = helm.InstallOrUpgrade(cn, "mariadb", "mariadb-development", "nicholaswilde/mariadb",
		[]string{
			"--create-namespace", "--wait",
			"--set", "fullnameOverride=development",
			"--set", "secret.MYSQL_ROOT_PASSWORD=mariadbpass",
			"--set", "persistence.config.enabled=true",
		},
	)
	if err != nil {
		fmt.Printf("[%s] unable to install mariadb-development: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func InstallDnsmasq() {
	cn := platform.ControllerClusterName()
	_, err := kube.ApplyTemplate(cn, "default", "dnsmasq.yml.tmpl", true)
	if err != nil {
		fmt.Printf("[%s] unable to install dnsmasq: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func InstallResolver() {
	dest := filepath.Join("/etc/resolver", platform.Hostname())
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

func RemoveResolver() {
	dest := filepath.Join("/etc/resolver", platform.Hostname())
	if _, err := exec.Command("rm", "-f", dest).Output(); err != nil {
		fmt.Println(err)
	}
}

func ConfigureKeycloak() {
	cn := platform.ControllerClusterName()
	if _, err := kube.ExecNoProgress(
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
	if out, err := kube.ExecNoProgress(
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
	_, err := kube.ExecNoProgress(cn, "lagoon-core", "lagoon-core-keycloak", `
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
func ConfigureTargetCoreDNS(cn string) {
	cm := kube.GetConfigMap(cn, "kube-system", "coredns")
	corednsCm := CoreDNSConfigMap{}
	err := json.Unmarshal(cm, &corednsCm)
	if err != nil {
		fmt.Printf("[%s] error parsing CoreDNS configmap: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	for _, h := range []string{"harbor", "broker", "ssh", "api", "gitea"} {
		entry := fmt.Sprintf("%s %s.lagoon.%s\n", k3d.ControllerIP(), h, platform.Hostname())
		if !strings.Contains(corednsCm.Data.NodeHosts, entry) {
			corednsCm.Data.NodeHosts += entry
		}
	}

	cm, err = json.Marshal(corednsCm)
	if err != nil {
		fmt.Printf("[%s] error encoding CoreDNS configmap: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	fmt.Printf("[%s] %s", cn, kube.Replace(cn, "kube-system", "coredns", string(cm)))
	out, err := kube.Cmd(cn, "kube-system", "rollout", "restart", "deployment/coredns").Output()
	if err != nil {
		fmt.Printf("[%s] CoreDNS restart failed: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	fmt.Printf("[%s] %s", cn, string(out))
}

func LagoonCliAddConfig() {
	graphql := fmt.Sprintf("http://api.lagoon.%s/graphql", platform.Hostname())
	ui := fmt.Sprintf("http://ui.lagoon.%s", platform.Hostname())

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
	cmd = exec.Command("lagoon", "config", "add", "--lagoon", platform.Name,
		"--graphql", graphql, "--ui", ui, "--hostname", "127.0.0.1", "--port", "2022")
	_, err = cmd.Output()
	if err != nil {
		panic(err)
	}
}

func LagoonCliDeleteConfig() {
	// Get list of existing configs.
	cmd := exec.Command("lagoon", "config", "delete", "--lagoon", platform.Name, "--force")
	_, err := cmd.Output()
	if err != nil {
		panic(err)
	}
}

func ClusterVersion(cn string) {
	_, err := kube.Cmd(cn, "", "version").Output()
	if err != nil {
		fmt.Printf("[%s] could not get cluster version: %s\n", cn, err)
	}
}

func Status() {
	k3d.ClusterFetch()
	if len(k3d.Clusters) == 0 {
		fmt.Printf("No cluster found for '%s'\n", platform.Name)
		return
	}

	fmt.Print("Registry: ")
	if k3d.Reg.State.Running {
		fmt.Println("running")
	} else {
		fmt.Println("stopped")
	}

	runningClusters := 0
	fmt.Println("Clusters:")
	for _, c := range k3d.Clusters {
		isRunning := k3d.ClusterIsRunning(c.Name)
		fmt.Printf("  %s: ", c.Name)
		if isRunning {
			fmt.Println("running")
			runningClusters++
		} else {
			fmt.Println("stopped")
		}
	}

	if runningClusters == 0 {
		fmt.Println("No running cluster")
		return
	}

	fmt.Println("Kubeconfig:")
	fmt.Println("  Controller:", internal.KubeconfigPath(platform.ControllerClusterName()))
	if len(k3d.Clusters) > 1 {
		fmt.Println("  Targets:")
		for _, c := range k3d.Clusters {
			if c.Name == platform.ControllerClusterName() {
				continue
			}
			fmt.Println("    ", internal.KubeconfigPath(c.Name))
		}
	}

	fmt.Println("Gitea:")
	fmt.Printf("  http://gitea.lagoon.%s\n", platform.Hostname())
	fmt.Println("  User: rockpool")
	fmt.Println("  Pass: pass")

	fmt.Println("Keycloak:")
	fmt.Printf("  http://keycloak.lagoon.%s/auth/admin\n", platform.Hostname())
	fmt.Println("  User: admin")
	fmt.Println("  Pass: pass")

	fmt.Printf("Lagoon UI: http://ui.lagoon.%s\n", platform.Hostname())
	fmt.Println("  User: lagoonadmin")
	fmt.Println("  Pass: pass")

	fmt.Printf("Lagoon GraphQL: http://api.lagoon.%s/graphql\n", platform.Hostname())
	fmt.Println("Lagoon SSH: ssh -p 2022 lagoon@localhost")

	fmt.Println()
}
