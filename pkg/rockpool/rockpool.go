package rockpool

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/salsadigitalauorg/rockpool/pkg/action"
	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/docker"
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
	log.WithField("clusters", clusters).Info("starting clusters")
	k3d.ClusterFetch()
	if len(clusters) == 0 {
		clusters = allClusters()
	}
	for _, cn := range clusters {
		k3d.ClusterStart(cn)
		AddHarborHostEntries(cn)
		if cn != platform.ControllerClusterName() {
			action.Handler{
				Stage:     "cluster-start",
				Info:      "configuring coredns for target",
				LogFields: log.Fields{"cluster": cn},
				Func:      ConfigureTargetCoreDNS,
			}.Execute()
		}
	}
}

func Stop(clusters []string) {
	log.WithField("clusters", clusters).Info("stopping clusters")
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
	log.WithField("clusters", clusters).Info("stopping and deleting clusters")
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

	chain.Add(kube.Applyer{
		Stage:       "controller-setup",
		Info:        "installing mailhog",
		ClusterName: clusterName,
		Namespace:   "default",
		Force:       true,
		Template:    "mailhog.yml.tmpl",
	})

	chain.Add(action.Handler{
		Func: func(logger *log.Entry) bool {
			helm.FetchInstalledReleases(clusterName)
			return true
		},
	})

	ingressNginxInstaller.Stage = "controller-setup"
	ingressNginxInstaller.ClusterName = clusterName
	chain.Add(ingressNginxInstaller)

	chain.Add(kube.Applyer{
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
	}).Add(kube.Applyer{
		Stage:       "controller-setup",
		ClusterName: clusterName,
		Namespace:   "cert-manager",
		Template:    "ca.yml.tmpl",
		Force:       true,
		Retries:     30,
		Delay:       10,
	})

	chain.Add(kube.Applyer{
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
		Info:        "installing gitea",
		ClusterName: clusterName,
		AddRepo: helm.HelmRepo{
			Name: "gitea-charts",
			Url:  "https://dl.gitea.io/charts/",
		},
		Namespace:          "gitea",
		ReleaseName:        "gitea",
		Chart:              "gitea-charts/gitea",
		Args:               []string{"--create-namespace", "--wait"},
		ValuesTemplate:     "gitea-values.yml.tmpl",
		ValuesTemplateVars: platform.ToMap(),
	}).Add(action.Handler{
		Func: func(logger *log.Entry) bool {
			// Create test repo.
			gitea.CreateRepo()
			return true
		},
	})

	chain.Add(helm.Installer{
		Stage:       "controller-setup",
		Info:        "installing harbor",
		ClusterName: clusterName,
		AddRepo: helm.HelmRepo{
			Name: "harbor",
			Url:  "https://helm.goharbor.io",
		},
		Namespace:          "harbor",
		ReleaseName:        "harbor",
		Chart:              "harbor/harbor",
		Args:               []string{"--create-namespace", "--wait", "--version=1.5.6"},
		ValuesTemplate:     "harbor-values.yml.tmpl",
		ValuesTemplateVars: platform.ToMap(),
	})

	lagoonValues := platform.ToMap()
	lagoonValues["LagoonVersion"] = lagoon.Version
	chain.Add(helm.Installer{
		Stage:       "controller-setup",
		Info:        "installing lagoon core",
		ClusterName: clusterName,
		AddRepo: helm.HelmRepo{
			Name: "lagoon",
			Url:  "https://uselagoon.github.io/lagoon-charts/",
		},
		Namespace:          "lagoon-core",
		ReleaseName:        "lagoon-core",
		Chart:              "lagoon/lagoon-core",
		Args:               []string{"--create-namespace", "--wait", "--timeout", "30m0s"},
		ValuesTemplate:     "lagoon-core-values.yml.tmpl",
		ValuesTemplateVars: lagoonValues,
	}).Add(action.Handler{
		Stage:     "controller-setup",
		Info:      "ensuring db tables have been created",
		LogFields: log.Fields{"cluster": clusterName},
		Func: func(logger *log.Entry) bool {
			cn := logger.Data["cluster"].(string)

			logger.Debug("checking if tables exist")
			out, err := kube.Cmd(cn, "lagoon-core", "exec",
				"sts/lagoon-core-api-db", "--", "bash", "-c",
				"mysql -u$MARIADB_USER -p$MARIADB_PASSWORD $MARIADB_DATABASE -e 'SHOW TABLES;'",
			).Output()
			if err != nil {
				logger.WithError(command.GetMsgFromCommandError(err)).
					Fatal("error getting tables")
			}
			if string(out) != "" {
				return true
			}

			logger.Debug("running the db init script")
			err = kube.Cmd(cn, "lagoon-core", "exec", "sts/lagoon-core-api-db",
				"--", "/legacy_rerun_initdb.sh").Run()
			if err != nil {
				logger.WithError(command.GetMsgFromCommandError(err)).
					Fatal("error running db init")
			}

			return true
		},
	})

	chain.Add(action.Handler{
		Stage:     "controller-setup",
		Info:      "configuring keycloak",
		LogFields: log.Fields{"cluster": clusterName},
		Func: func(logger *log.Entry) bool {
			logger.Debug("logging into keycloak")
			cn := logger.Data["cluster"].(string)
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
				logger.WithError(command.GetMsgFromCommandError(err)).
					Fatal("error logging in to Keycloak")
			}

			logger.Debug("checking if keycloak has already been configured")
			if out, err := kube.Exec(
				cn, "lagoon-core", "lagoon-core-keycloak", `
set -e
/opt/jboss/keycloak/bin/kcadm.sh get realms/lagoon \
	--fields 'smtpServer(from)' --config /tmp/kcadm.config
`,
			).Output(); err != nil {
				logger.WithError(command.GetMsgFromCommandError(err)).
					Fatal("error checking keycloak configuration")
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
					return true
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
				logger.WithError(command.GetMsgFromCommandError(err)).
					Fatal("error configuring keycloak")
			}
			return true
		},
	})

	chain.Add(action.Handler{
		Stage:     "controller-setup",
		Info:      "configuring lagoon client",
		LogFields: log.Fields{"cluster": clusterName},
		Func: func(logger *log.Entry) bool {
			lagoon.InitApiClient()
			lagoon.AddSshKey()
			LagoonCliAddConfig()
			return true
		},
	})

	chain.Run()
}

func SetupLagoonTarget(clusterName string) {
	chain := action.Chain{}

	defer platform.WgDone()

	chain.Add(action.Handler{
		Stage:     "target-setup",
		Info:      "configuring coredns for target",
		LogFields: log.Fields{"cluster": clusterName},
		Func:      ConfigureTargetCoreDNS,
	})

	chain.Add(action.Handler{
		Func: func(logger *log.Entry) bool {
			helm.FetchInstalledReleases(clusterName)
			return true
		},
	})
	ingressNginxInstaller.Stage = "target-setup"
	ingressNginxInstaller.ClusterName = clusterName
	chain.Add(ingressNginxInstaller)

	chain.Add(helm.Installer{
		Stage:       "target-setup",
		Info:        "installing nfs provisioner",
		ClusterName: clusterName,
		AddRepo: helm.HelmRepo{
			Name: "nfs-provisioner",
			Url:  "https://kubernetes-sigs.github.io/nfs-ganesha-server-and-external-provisioner/",
		},
		Namespace:          "nfs-provisioner",
		ReleaseName:        "nfs",
		Chart:              "nfs-provisioner/nfs-server-provisioner",
		Args:               []string{"--create-namespace", "--wait"},
		ValuesTemplate:     "nfs-server-provisioner-values.yml.tmpl",
		ValuesTemplateVars: platform.ToMap(),
	})

	chain.Add(kube.Applyer{
		Stage:       "target-setup",
		Info:        "applying dbaas-operator manifests",
		ClusterName: clusterName,
		Namespace:   "",
		Urls: []string{
			"https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/mariadb.yaml",
			"https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/mongodb.yaml",
			"https://raw.githubusercontent.com/amazeeio/charts/main/charts/dbaas-operator/crds/postgres.yaml",
		},
		Force: true,
	}).Add(helm.Installer{
		Stage:       "target-setup",
		Info:        "installing mariadb-production",
		ClusterName: clusterName,
		AddRepo: helm.HelmRepo{
			Name: "nicholaswilde",
			Url:  "https://nicholaswilde.github.io/helm-charts/",
		},
		Namespace:   "mariadb",
		ReleaseName: "mariadb-production",
		Chart:       "nicholaswilde/mariadb",
		Args: []string{
			"--create-namespace", "--wait",
			"--set", "fullnameOverride=production",
			"--set", "secret.MYSQL_ROOT_PASSWORD=mariadbpass",
			"--set", "persistence.config.enabled=true",
		},
	}).Add(helm.Installer{
		Stage:       "target-setup",
		Info:        "installing mariadb-development",
		ClusterName: clusterName,
		AddRepo: helm.HelmRepo{
			Name: "nicholaswilde",
			Url:  "https://nicholaswilde.github.io/helm-charts/",
		},
		Namespace:   "mariadb",
		ReleaseName: "mariadb-development",
		Chart:       "nicholaswilde/mariadb",
		Args: []string{
			"--create-namespace", "--wait",
			"--set", "fullnameOverride=development",
			"--set", "secret.MYSQL_ROOT_PASSWORD=mariadbpass",
			"--set", "persistence.config.enabled=true",
		},
	})

	lagoonValues := platform.ToMap()
	lagoonValues["LagoonVersion"] = lagoon.Version
	lagoonValues["TargetId"] = fmt.Sprint(kube.GetTargetIdFromCn(clusterName))
	_, lagoonValues["RabbitMQPassword"] = kube.GetSecret(platform.ControllerClusterName(),
		"lagoon-core",
		"lagoon-core-broker",
		"RABBITMQ_PASSWORD",
	)
	chain.Add(helm.Installer{
		Stage:       "target-setup",
		Info:        "installing lagoon remote",
		ClusterName: clusterName,
		AddRepo: helm.HelmRepo{
			Name: "lagoon",
			Url:  "https://uselagoon.github.io/lagoon-charts/",
		},
		Namespace:          "lagoon",
		ReleaseName:        "lagoon-remote",
		Chart:              "lagoon/lagoon-remote",
		Args:               []string{"--create-namespace", "--wait"},
		ValuesTemplate:     "lagoon-remote-values.yml.tmpl",
		ValuesTemplateVars: lagoonValues,
	})

	chain.Add(action.Handler{
		Stage:     "target-setup",
		Info:      "registering lagoon remote",
		LogFields: log.Fields{"cluster": clusterName},
		Func: func(logger *log.Entry) bool {
			cn := logger.Data["cluster"].(string)
			cId := kube.GetTargetIdFromCn(cn)
			rName := platform.Name + fmt.Sprint(cId)
			re := lagoon.Remote{
				Id:            cId,
				Name:          rName,
				ConsoleUrl:    fmt.Sprintf("https://%s:6443", k3d.TargetIP(cn)),
				RouterPattern: fmt.Sprintf("${environment}.${project}.%s.%s", rName, platform.Hostname()),
			}
			for _, existingRe := range lagoon.Remotes {
				if existingRe.Id == re.Id && existingRe.Name == re.Name {
					logger.WithField("remote", re.Name).Debug("Lagoon remote already exists")
					return true
				}
			}
			b64Token, err := kube.Cmd(cn, "lagoon", "get", "secret",
				"-o=jsonpath='{.items[?(@.metadata.annotations.kubernetes\\.io/service-account\\.name==\"lagoon-remote-kubernetes-build-deploy\")].data.token}'").Output()
			if err != nil {
				logger.WithError(command.GetMsgFromCommandError(err)).
					Fatal("error fetching lagoon remote token")
			}
			token, err := base64.URLEncoding.DecodeString(strings.Trim(string(b64Token), "'"))
			if err != nil {
				logger.WithError(err).Fatal("error decoding lagoon remote token")
			}
			lagoon.AddRemote(re, string(token))
			return true
		},
	})

	chain.Run()
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

func InstallResolver() {
	nameserverIp := docker.GetVmIp()

	dest := filepath.Join("/etc/resolver", platform.Hostname())
	logger := log.WithField("resolverFile", dest)
	logger.Info("installing resolver file")

	data := fmt.Sprintf(`
nameserver %s
port 6153
`, nameserverIp)

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
		}).WithError(command.GetMsgFromCommandError(err)).
			Panic("unable to move file")
	}
}

func RemoveResolver() {
	dest := filepath.Join("/etc/resolver", platform.Hostname())
	logger := log.WithField("resolverFile", dest)
	logger.Info("removing resolver file")
	if err := command.ShellCommander("rm", "-f", dest).Run(); err != nil {
		logger.WithError(command.GetMsgFromCommandError(err)).
			Warn("error when deleting resolver file")
	}
}

func LagoonCliAddConfig() {
	graphql := fmt.Sprintf("http://api.lagoon.%s/graphql", platform.Hostname())
	ui := fmt.Sprintf("http://ui.lagoon.%s", platform.Hostname())

	// Get list of existing configs.
	out, err := command.ShellCommander("lagoon", "config", "list",
		"--output-json").Output()
	if err != nil {
		log.WithError(command.GetMsgFromCommandError(err)).
			Panic("could not get lagoon configs")
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
		log.WithError(command.GetMsgFromCommandError(err)).
			Panic("could not add lagoon config")
	}
}

func LagoonCliDeleteConfig() {
	// Get list of existing configs.
	err := command.ShellCommander("lagoon", "config", "delete", "--lagoon",
		platform.Name, "--force").Run()
	if err != nil {
		log.WithError(command.GetMsgFromCommandError(err)).
			Panic("could not delete lagoon config")
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
