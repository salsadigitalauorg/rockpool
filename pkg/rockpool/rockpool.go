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
	log.Debug("checking if binaries exist")
	binaries := []string{"k3d", "docker", "kubectl", "helm", "lagoon"}
	missing := []string{}
	versionError := false
	for _, b := range binaries {
		absPath, err := exec.LookPath(b)
		if err != nil {
			missing = append(missing, fmt.Sprintf("could not find %s; please ensure it is installed and can be found in the $PATH", b))
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
		log.Fatal("some requirements were not met; please review above")
	}
}

func Initialise() {
	EnsureBinariesExist()

	Spinner = *spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	Spinner.Color("red", "bold")

	// Create directory for rendered templates.
	templDir := templates.RenderedPath(true)
	log.WithField("dir", templDir).Debug("creating directory for rendered templates")
	err := os.MkdirAll(templDir, os.ModePerm)
	if err != nil {
		log.WithFields(log.Fields{
			"dir": templDir,
			"err": err,
		}).Fatal("unable to create temp dir")
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
	logger := log.WithField("clusterName", cn)
	logger.Info("installing mailhog")
	kube.ApplyTemplate(cn, "default", "mailhog.yml.tmpl", true, 0, 0)
}

func SetupNginxReverseProxyForRemotes() {
	cn := platform.ControllerClusterName()
	logger := log.WithField("clusterName", cn)
	logger.Info("setting up nginx reverse proxy for remotes")

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
		logger.WithField("err", err).Fatal("error rendering template")
	}

	kube.Apply(cn, "ingress-nginx", patchFile, true)
}

func InstallCertManager() {
	cn := platform.ControllerClusterName()
	logger := log.WithField("clusterName", cn)
	logger.Info("installing cert-manager")

	kube.ApplyTemplate(cn, "", "cert-manager.yaml", true, 0, 0)

	retries := 10
	deployNotFound := true
	var failedErr error
	for deployNotFound && retries > 0 {
		failedErr = nil
		_, err := kube.Cmd(
			platform.ControllerClusterName(), "cert-manager",
			"wait", "--for=condition=Available=true",
			"deployment/cert-manager-webhook").Output()
		if err != nil {
			failedErr = err
			retries--
			time.Sleep(5 * time.Second)
			continue
		}
		deployNotFound = false
	}
	if failedErr != nil {
		logger.WithField("err", command.GetMsgFromCommandError(failedErr)).
			Fatal("error while waiting for cert-manager webhook")
	}

	kube.ApplyTemplate(platform.ControllerClusterName(), "cert-manager",
		"ca.yml.tmpl", true, 30, 10)
}

func InstallGitlab() {
	cn := platform.ControllerClusterName()
	log.WithField("clusterName", cn).Info("installing gitlab")
	kube.ApplyTemplate(cn, "gitlab", "gitlab.yml.tmpl", true, 0, 0)
}

func InstallGitea() {
	cn := platform.ControllerClusterName()
	logger := log.WithField("clusterName", cn)
	logger.Info("installing gitea")

	err := helm.Exec(cn, "", "repo", "add", "gitea-charts",
		"https://dl.gitea.io/charts/").Run()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("error adding gitea helm repo")
	}

	values, err := templates.Render("gitea-values.yml.tmpl", platform.ToMap(), "")
	if err != nil {
		logger.WithField("err", err).
			Fatal("error rendering gitea values template")
	}

	err = helm.InstallOrUpgrade(cn, "gitea", "gitea", "gitea-charts/gitea",
		[]string{"--create-namespace", "--wait", "-f", values},
	)
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to install gitea")
	}
}

func InstallNfsProvisioner(cn string) {
	logger := log.WithField("clusterName", cn)
	logger.Info("installing nfs provisioner")

	err := helm.Exec(cn, "", "repo", "add", "nfs-provisioner",
		"https://kubernetes-sigs.github.io/nfs-ganesha-server-and-external-provisioner/").Run()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to add nfs-provisioner repo")
	}

	values, err := templates.Render("nfs-server-provisioner-values.yml.tmpl",
		platform.ToMap(), "")
	if err != nil {
		logger.WithField("err", err).
			Fatal("error rendering nfs-provisioner values template")
	}

	err = helm.InstallOrUpgrade(cn, "nfs-provisioner", "nfs",
		"nfs-provisioner/nfs-server-provisioner",
		[]string{"--create-namespace", "--wait", "-f", values},
	)
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to install nfs-provisioner")
	}
}

func InstallMariaDB(cn string) {
	logger := log.WithField("clusterName", cn)
	logger.Info("installing mariadb")

	err := helm.Exec(cn, "", "repo", "add", "nicholaswilde",
		"https://nicholaswilde.github.io/helm-charts/").Run()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to add nicholaswilde repo")
	}

	err = kube.Apply(cn, "", "https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/mariadb.yaml", true)
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to install mariadb crds")
	}

	err = kube.Apply(cn, "", "https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/mongodb.yaml", true)
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to install mongodb crds")
	}

	err = kube.Apply(cn, "", "https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/postgres.yaml", true)
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to install postgres crds")
	}

	err = helm.InstallOrUpgrade(cn, "mariadb", "mariadb-production", "nicholaswilde/mariadb",
		[]string{
			"--create-namespace", "--wait",
			"--set", "fullnameOverride=production",
			"--set", "secret.MYSQL_ROOT_PASSWORD=mariadbpass",
			"--set", "persistence.config.enabled=true",
		},
	)
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to install mariadb-production")
	}

	err = helm.InstallOrUpgrade(cn, "mariadb", "mariadb-development", "nicholaswilde/mariadb",
		[]string{
			"--create-namespace", "--wait",
			"--set", "fullnameOverride=development",
			"--set", "secret.MYSQL_ROOT_PASSWORD=mariadbpass",
			"--set", "persistence.config.enabled=true",
		},
	)
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("unable to install mariadb-development")
	}
}

func InstallDnsmasq() {
	cn := platform.ControllerClusterName()
	log.WithField("clusterName", cn).Info("installing dnsmasq")
	kube.ApplyTemplate(cn, "default", "dnsmasq.yml.tmpl", true, 0, 0)
}

func InstallResolver() {
	dest := filepath.Join("/etc/resolver", platform.Hostname())
	logger := log.WithField("resolverFile", dest)
	logger.Info("installing resolver file")

	data := `
nameserver 127.0.0.1
port 6153
`

	var tmpFile *os.File
	var err error

	if _, err := os.Stat(dest); err == nil {
		logger.Debug("resolver file already exists")
		return
	}

	logger.Info("creating resolver file")
	if tmpFile, err = ioutil.TempFile("", "rockpool-resolver-"); err != nil {
		logger.WithField("err", err).Panic("unable to create temporary file")
	}
	if err = os.Chmod(tmpFile.Name(), 0777); err != nil {
		logger.WithFields(log.Fields{
			"tempFile": tmpFile.Name(),
			"err":      err,
		}).Panic("unable to set file permissions")
	}
	if _, err = tmpFile.WriteString(data); err != nil {
		logger.WithFields(log.Fields{
			"tempFile": tmpFile.Name(),
			"err":      err,
		}).Panic("unable to write to temporary file")
	}
	if _, err = exec.Command("sudo", "mv", tmpFile.Name(), dest).Output(); err != nil {
		logger.WithFields(log.Fields{
			"tempFile":    tmpFile.Name(),
			"destination": dest,
			"err":         command.GetMsgFromCommandError(err),
		}).Panic("unable to move file")
	}
}

func RemoveResolver() {
	dest := filepath.Join("/etc/resolver", platform.Hostname())
	logger := log.WithField("resolverFile", dest)
	logger.Info("removing resolver file")
	if err := command.ShellCommander("rm", "-f", dest).Run(); err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Warn("error when deleting resolver file")
	}
}

func ConfigureKeycloak() {
	cn := platform.ControllerClusterName()
	logger := log.WithField("clusterName", cn)
	logger.Info("configuring keycloak")

	if err := kube.Exec(
		cn, "lagoon-core", "lagoon-core-keycloak", `
set -e
rm -f /tmp/kcadm.config
/opt/jboss/keycloak/bin/kcadm.sh config credentials \
  --server http://localhost:8080/auth --realm master \
  --user $KEYCLOAK_ADMIN_USER --password $KEYCLOAK_ADMIN_PASSWORD \
  --config /tmp/kcadm.config
`,
	).Run(); err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("error logging in to Keycloak")
	}

	// Skip if values have already been set.
	if out, err := kube.Exec(
		cn, "lagoon-core", "lagoon-core-keycloak", `
set -e
/opt/jboss/keycloak/bin/kcadm.sh get realms/lagoon \
	--fields 'smtpServer(from)' --config /tmp/kcadm.config
`,
	).Output(); err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("error checking keycloak configuration")
	} else {
		s := struct {
			SmtpServer struct {
				From string `json:"from"`
			} `json:"smtpServer"`
		}{}
		err := json.Unmarshal(out, &s)
		if err != nil {
			logger.WithField("err", err).
				Fatal("error parsing keycloak configuration")
		}
		if s.SmtpServer.From == "lagoon@k3d-rockpool" {
			logger.Debug("keycloak already configured")
			return
		}
	}

	// Configure keycloak.
	err := kube.Exec(cn, "lagoon-core", "lagoon-core-keycloak", `
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
	).Run()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("error configuring keycloak")
	}
}

// ConfigureTargetCoreDNS adds DNS records to targets for the required services.
func ConfigureTargetCoreDNS(cn string) {
	logger := log.WithField("clusterName", cn)
	logger.Info("configuring coredns for target")

	cm := kube.GetConfigMap(cn, "kube-system", "coredns")
	corednsCm := CoreDNSConfigMap{}
	err := json.Unmarshal(cm, &corednsCm)
	if err != nil {
		logger.WithField("err", err).Fatal("error parsing CoreDNS configmap")
	}
	for _, h := range []string{"harbor", "broker", "ssh", "api", "gitea"} {
		entry := fmt.Sprintf("%s %s.lagoon.%s\n", k3d.ControllerIP(), h, platform.Hostname())
		if !strings.Contains(corednsCm.Data.NodeHosts, entry) {
			corednsCm.Data.NodeHosts += entry
		}
	}

	cm, err = json.Marshal(corednsCm)
	if err != nil {
		logger.WithField("err", err).Fatal("error encoding CoreDNS configmap")
	}

	kube.Replace(cn, "kube-system", "coredns", string(cm))

	logger.Info("restarting coredns")
	err = kube.Cmd(cn, "kube-system", "rollout", "restart",
		"deployment/coredns").RunProgressive()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("CoreDNS restart failed")
	}
}

func LagoonCliAddConfig() {
	graphql := fmt.Sprintf("http://api.lagoon.%s/graphql", platform.Hostname())
	ui := fmt.Sprintf("http://ui.lagoon.%s", platform.Hostname())

	// Get list of existing configs.
	out, err := command.ShellCommander("lagoon", "config", "list",
		"--output-json").Output()
	if err != nil {
		log.WithField("err", err).Panic("could not get lagoon configs")
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
		log.WithField("err", err).Panic("could not parse lagoon configs")
	}

	logger := log.WithFields(log.Fields{
		"name":     platform.Name,
		"hostname": platform.Hostname(),
		"graphql":  graphql,
		"ui":       ui,
	})

	// Add the config.
	logger.Info("adding lagoon config")
	err = command.ShellCommander("lagoon", "config", "add", "--lagoon",
		platform.Name, "--graphql", graphql, "--ui", ui, "--hostname",
		"127.0.0.1", "--port", "2022").Run()
	if err != nil {
		log.WithField("err", err).Panic("could not add lagoon config")
	}
}

func LagoonCliDeleteConfig() {
	// Get list of existing configs.
	err := command.ShellCommander("lagoon", "config", "delete", "--lagoon",
		platform.Name, "--force").Run()
	if err != nil {
		log.WithField("err", err).Panic("could not delete lagoon config")
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
