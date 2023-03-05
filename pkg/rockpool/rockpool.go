package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/gitea"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
	"github.com/salsadigitalauorg/rockpool/pkg/k3d"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
	"github.com/salsadigitalauorg/rockpool/pkg/lagoon"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	"github.com/salsadigitalauorg/rockpool/pkg/platform/templates"

	log "github.com/sirupsen/logrus"
)

func EnsureBinariesExist() {
	log.Debug("checking if binaries exist")
	chain := &action.Chain{
		FailOnFirstError: &[]bool{false}[0],
		ErrorMsg:         "some requirements were not met; please review above",
	}
	chain.Add(action.BinaryExists{Bin: "k3d"}).
		Add(action.BinaryExists{Bin: "docker", VersionArgs: []string{"--format", "json"}}).
		Add(action.BinaryExists{Bin: "kubectl", VersionArgs: []string{"--client", "--short"}}).
		Add(action.BinaryExists{Bin: "helm"}).
		Add(action.BinaryExists{Bin: "lagoon"}).
		Run()
}

func Initialise() {
	EnsureBinariesExist()

	// Create directory for rendered templates.
	templDir := templates.RenderedPath(true)
	log.WithField("dir", templDir).Debug("creating directory for rendered templates")
	err := os.MkdirAll(templDir, os.ModePerm)
	if err != nil {
		log.WithField("dir", templDir).WithError(err).
			Fatal("unable to create temp dir")
	}
}

func Up(desiredClusters []string) {
	k3d.ClusterFetch()
	if len(desiredClusters) == 0 {
		if len(k3d.Clusters) > 0 {
			desiredClusters = allClusters()
		} else {
			desiredClusters = append(desiredClusters, platform.ControllerClusterName())
			for i := 1; i <= platform.NumTargets; i++ {
				desiredClusters = append(desiredClusters, platform.TargetClusterName(i))
			}
		}
	}
	k3d.RegistryCreate()
	k3d.RegistryRenderConfig()
	k3d.RegistryStart()
	CreateClusters(desiredClusters)

	setupController := false
	setupTargets := []string{}
	for _, c := range desiredClusters {
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
	log.WithField("clusters", clusters).Info("stopping all clusters")
	k3d.ClusterFetch()
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
	log.WithField("clusters", clusters).Info("stopping and deleting all clusters")
	k3d.ClusterFetch()
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
	clusterName := platform.ControllerClusterName()

	chain := action.Chain{}

	chain.Add(kube.Templater{
		Stage:       "controller-setup",
		Info:        "installing mailhog",
		ClusterName: clusterName,
		Namespace:   "default",
		Force:       true,
		Template:    "mailhog.yml.tmpl",
	})

	chain.Add(action.Handler{
		Func: func(logger *log.Entry) bool {
			helm.List(clusterName)
			return true
		},
	})

	ingressNginxInstaller.Stage = "controller-setup"
	ingressNginxInstaller.ClusterName = clusterName
	chain.Add(ingressNginxInstaller)

	chain.Add(kube.Templater{
		Stage:       "controller-setup",
		Info:        "installing cert-manager",
		ClusterName: clusterName,
		Namespace:   "",
		Template:    "cert-manager.yaml",
		Force:       true,
	}).Add(kube.Waiter{
		Stage:       "controller-setup",
		ClusterName: clusterName,
		Namespace:   "cert-manager",
		Resource:    "deployment/cert-manager-webhook",
		Condition:   "Available=true",
		Retries:     10,
		Delay:       5,
	}).Add(kube.Templater{
		Stage:       "controller-setup",
		ClusterName: clusterName,
		Namespace:   "cert-manager",
		Template:    "ca.yml.tmpl",
		Force:       true,
		Retries:     30,
		Delay:       10,
	})

	chain.Add(kube.Templater{
		Stage:       "controller-setup",
		Info:        "installing dnsmasq",
		ClusterName: clusterName,
		Namespace:   "default",
		Force:       true,
		Template:    "dnsmasq.yml.tmpl",
	})

	// chain.Add(kube.Templater{
	// 	Stage:       "controller-setup",
	// 	Info:        "installing gitlab",
	// 	ClusterName: clusterName,
	// 	Namespace:   "gitlab",
	// 	Force:       true,
	// 	Template:    "gitlab.yml.tmpl",
	// })

	chain.Add(helm.Installer{
		Stage:       "controller-setup",
		ClusterName: clusterName,
		Namespace:   "gitea",
		AddRepo: helm.HelmRepo{
			Name: "gitea-charts",
			Url:  "https://dl.gitea.io/charts/",
		},
		ReleaseName:        "gitea",
		Chart:              "gitea-charts/gitea",
		Args:               []string{"--create-namespace"},
		Info:               "installing gitea",
		ValuesTemplate:     "gitea-values.yml.tmpl",
		ValuesTemplateVars: platform.ToMap(),
	})

	chain.Run()

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

func SetupLagoonTarget(clusterName string) {
	chain := action.Chain{}

	defer platform.WgDone()

	helm.List(clusterName)
	ConfigureTargetCoreDNS(clusterName)

	ingressNginxInstaller.Stage = "target-setup"
	ingressNginxInstaller.ClusterName = clusterName
	chain.Add(ingressNginxInstaller)

	chain.Run()

	InstallNfsProvisioner(clusterName)
	InstallMariaDB(clusterName)
	InstallLagoonRemote(clusterName)
	RegisterLagoonRemote(clusterName)
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
		logger.WithError(err).Fatal("error rendering template")
	}

	kube.Apply(cn, "ingress-nginx", patchFile, true)
}

func InstallNfsProvisioner(cn string) {
	logger := log.WithField("clusterName", cn)
	logger.Info("installing nfs provisioner")

	err := helm.Exec(cn, "", "repo", "add", "nfs-provisioner",
		"https://kubernetes-sigs.github.io/nfs-ganesha-server-and-external-provisioner/").Run()
	if err != nil {
		logger.WithError(err).Fatal("unable to add nfs-provisioner repo")
	}

	values, err := templates.Render("nfs-server-provisioner-values.yml.tmpl",
		platform.ToMap(), "")
	if err != nil {
		logger.WithError(err).Fatal("error rendering nfs-provisioner values template")
	}

	err = helm.InstallOrUpgrade(cn, "nfs-provisioner", "nfs",
		"nfs-provisioner/nfs-server-provisioner",
		[]string{"--create-namespace", "--wait", "-f", values},
	)
	if err != nil {
		logger.WithError(err).Fatal("unable to install nfs-provisioner")
	}
}

func InstallMariaDB(cn string) {
	logger := log.WithField("clusterName", cn)
	logger.Info("installing mariadb")

	err := helm.Exec(cn, "", "repo", "add", "nicholaswilde",
		"https://nicholaswilde.github.io/helm-charts/").Run()
	if err != nil {
		logger.WithError(err).Fatal("unable to add nicholaswilde repo")
	}

	err = kube.Apply(cn, "", "https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/mariadb.yaml", true)
	if err != nil {
		logger.WithError(err).Fatal("unable to install mariadb crds")
	}

	err = kube.Apply(cn, "", "https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/mongodb.yaml", true)
	if err != nil {
		logger.WithError(err).Fatal("unable to install mongodb crds")
	}

	err = kube.Apply(cn, "", "https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/postgres.yaml", true)
	if err != nil {
		logger.WithError(err).Fatal("unable to install postgres crds")
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
		logger.WithError(err).Fatal("unable to install mariadb-production")
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
		logger.WithError(err).Fatal("unable to install mariadb-development")
	}
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
	if tmpFile, err = os.CreateTemp("", "rockpool-resolver-"); err != nil {
		logger.WithError(err).Panic("unable to create temporary file")
	}
	if err = os.Chmod(tmpFile.Name(), 0777); err != nil {
		logger.WithField("tempFile", tmpFile.Name()).WithError(err).
			Panic("unable to set file permissions")
	}
	if _, err = tmpFile.WriteString(data); err != nil {
		logger.WithField("tempFile", tmpFile.Name()).WithError(err).
			Panic("unable to write to temporary file")
	}
	if err = command.ShellCommander("sudo", "mv", tmpFile.Name(), dest).Run(); err != nil {
		logger.WithFields(log.Fields{
			"tempFile":    tmpFile.Name(),
			"destination": dest,
		}).WithError(err).Panic("unable to move file")
	}
}

func RemoveResolver() {
	dest := filepath.Join("/etc/resolver", platform.Hostname())
	logger := log.WithField("resolverFile", dest)
	logger.Info("removing resolver file")
	if err := command.ShellCommander("rm", "-f", dest).Run(); err != nil {
		logger.WithError(err).Warn("error when deleting resolver file")
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
		logger.WithError(err).Fatal("error logging in to Keycloak")
	}

	// Skip if values have already been set.
	if out, err := kube.Exec(
		cn, "lagoon-core", "lagoon-core-keycloak", `
set -e
/opt/jboss/keycloak/bin/kcadm.sh get realms/lagoon \
	--fields 'smtpServer(from)' --config /tmp/kcadm.config
`,
	).Output(); err != nil {
		logger.WithError(err).Fatal("error checking keycloak configuration")
	} else {
		s := struct {
			SmtpServer struct {
				From string `json:"from"`
			} `json:"smtpServer"`
		}{}
		err := json.Unmarshal(out, &s)
		if err != nil {
			logger.WithError(err).Fatal("error parsing keycloak configuration")
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
		logger.WithError(err).Fatal("error configuring keycloak")
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
		logger.WithError(err).Fatal("error parsing CoreDNS configmap")
	}
	for _, h := range []string{"harbor", "broker", "ssh", "api", "gitea"} {
		entry := fmt.Sprintf("%s %s.lagoon.%s\n", k3d.ControllerIP(), h, platform.Hostname())
		if !strings.Contains(corednsCm.Data.NodeHosts, entry) {
			corednsCm.Data.NodeHosts += entry
		}
	}

	cm, err = json.Marshal(corednsCm)
	if err != nil {
		logger.WithError(err).Fatal("error encoding CoreDNS configmap")
	}

	kube.Replace(cn, "kube-system", "coredns", string(cm))

	logger.Info("restarting coredns")
	err = kube.Cmd(cn, "kube-system", "rollout", "restart",
		"deployment/coredns").RunProgressive()
	if err != nil {
		logger.WithError(err).Fatal("CoreDNS restart failed")
	}
}

func LagoonCliAddConfig() {
	graphql := fmt.Sprintf("http://api.lagoon.%s/graphql", platform.Hostname())
	ui := fmt.Sprintf("http://ui.lagoon.%s", platform.Hostname())

	// Get list of existing configs.
	out, err := command.ShellCommander("lagoon", "config", "list",
		"--output-json").Output()
	if err != nil {
		log.WithError(err).Panic("could not get lagoon configs")
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
		log.WithError(err).Panic("could not parse lagoon configs")
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
		log.WithError(err).Panic("could not add lagoon config")
	}
}

func LagoonCliDeleteConfig() {
	// Get list of existing configs.
	err := command.ShellCommander("lagoon", "config", "delete", "--lagoon",
		platform.Name, "--force").Run()
	if err != nil {
		log.WithError(err).Panic("could not delete lagoon config")
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
	fmt.Println("  Controller:", kube.KubeconfigPath(platform.ControllerClusterName()))
	if len(k3d.Clusters) > 1 {
		fmt.Println("  Targets:")
		for _, c := range k3d.Clusters {
			if c.Name == platform.ControllerClusterName() {
				continue
			}
			fmt.Println("    ", kube.KubeconfigPath(c.Name))
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
