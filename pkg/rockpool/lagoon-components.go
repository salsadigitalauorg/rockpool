package rockpool

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/salsadigitalauorg/rockpool/internal"
	"github.com/salsadigitalauorg/rockpool/pkg/docker"
	"github.com/salsadigitalauorg/rockpool/pkg/helm"
	"github.com/salsadigitalauorg/rockpool/pkg/k3d"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	"github.com/salsadigitalauorg/rockpool/pkg/templates"
)

func (r *Rockpool) InstallIngressNginx(cn string) {
	_, err := helm.InstallOrUpgrade(cn, "ingress-nginx", "ingress-nginx",
		"https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-3.40.0/ingress-nginx-3.40.0.tgz",
		[]string{
			"--create-namespace", "--wait",
			"--set", "controller.config.ssl-redirect=false",
			"--set", "controller.config.proxy-body-size=8m",
			"--set", "server-name-hash-bucket-size=128",
		},
	)
	if err != nil {
		fmt.Printf("[%s] unable to install ingress-nginx: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallHarbor() {
	cn := r.ControllerClusterName()
	cmd := helm.Exec(cn, "", "repo", "add", "harbor", "https://helm.goharbor.io")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("[%s] unable to add harbor repo: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	values, err := templates.Render("harbor-values.yml.tmpl", platform.ToMap(), "")
	if err != nil {
		fmt.Printf("[%s] error rendering harbor values template: %s\n", cn, err)
		os.Exit(1)
	}
	fmt.Printf("[%s] using generated harbor values at %s\n", cn, values)

	_, err = helm.InstallOrUpgrade(cn,
		"harbor", "harbor", "harbor/harbor",
		[]string{
			"--create-namespace", "--wait",
			"-f", values, "--version=1.5.6",
		},
	)
	if err != nil {
		fmt.Printf("[%s] unable to install harbor: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) FetchHarborCerts() {
	cn := r.ControllerClusterName()
	certBytes, _ := kube.GetSecret(cn, "harbor", "harbor-harbor-ingress", "")
	certData := struct {
		Data map[string]string `json:"data"`
	}{}
	json.Unmarshal(certBytes, &certData)

	secretManifest, err := templates.Render("harbor-cert.yml.tmpl", certData, "")
	if err != nil {
		fmt.Printf("[%s] error rendering harbor cert template: %s\n", cn, err)
		os.Exit(1)
	}
	fmt.Printf("[%s] generated harbor cert at %s\n", cn, secretManifest)

	cacrt := certData.Data["ca.crt"]
	decoded, err := base64.URLEncoding.DecodeString(cacrt)
	if err != nil {
		fmt.Printf("[%s] error when decoding ca.crt: %#v\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	caCrtFile, err := templates.Render("harbor-ca.crt.tmpl", string(decoded), "")
	if err != nil {
		fmt.Printf("[%s] error rendering harbor ca.crt template: %s\n", cn, err)
		os.Exit(1)
	}
	fmt.Printf("[%s] generated harbor ca.crt at %s\n", cn, caCrtFile)

	r.State.HarborSecretManifest = secretManifest
	r.State.HarborCaCrtFile = caCrtFile
}

func (r *Rockpool) InstallHarborCerts(cn string) {
	if cn == r.ControllerClusterName() {
		return
	}

	exists, c := k3d.ClusterExists(cn)
	if !exists {
		fmt.Printf("[%s] cluster does not exist\n", cn)
		return
	}

	if _, err := kube.Apply(cn, "lagoon", r.State.HarborSecretManifest, true); err != nil {
		fmt.Printf("[%s] error creating ca.crt: %s\n", cn, err)
		os.Exit(1)
	}

	// Add the cert to the nodes.
	clusterUpdated := false
	for _, n := range c.Nodes {
		if n.Role == "loadbalancer" {
			continue
		}

		caCrtFileOut, _ := docker.Exec(n.Name, "ls /etc/ssl/certs/harbor-cert.crt")
		if strings.Trim(string(caCrtFileOut), "\n") == "/etc/ssl/certs/harbor-cert.crt" {
			continue
		}

		// Add harbor's ca.crt to the target.
		destCaCrt := fmt.Sprintf("%s:/etc/ssl/certs/harbor-cert.crt", n.Name)
		_, err := docker.Cp(r.State.HarborCaCrtFile, destCaCrt)
		if err != nil {
			fmt.Printf("[%s] error copying ca.crt: %s\n", cn, internal.GetCmdStdErr(err))
			os.Exit(1)
		}
		clusterUpdated = true
	}

	if clusterUpdated {
		k3d.ClusterRestart(c.Name)
	}

	// Patch lagoon-remote-lagoon-build-deploy to add the cert secret.
	patchFile, err := templates.Render("patch-lagoon-remote-lagoon-build-deploy.yaml", nil, "")
	if err != nil {
		fmt.Printf("[%s] error rendering the build deploy patch file: %s\n", cn, err)
		os.Exit(1)
	}
	_, err = kube.Patch(cn, "lagoon", "deployment", "lagoon-remote-lagoon-build-deploy", patchFile)
	if err != nil {
		fmt.Printf("[%s] error patching the lagoon-build-deploy deployment: %s\n", cn, err)
		os.Exit(1)
	}
}

// AddHarborHostEntries adds host entries to the target nodes.
func (r *Rockpool) AddHarborHostEntries(cn string) {
	if cn == r.ControllerClusterName() {
		return
	}

	exists, c := k3d.ClusterExists(cn)
	if !exists {
		fmt.Printf("[%s] cluster does not exist\n", cn)
		return
	}

	entry := fmt.Sprintf("%s\tharbor.lagoon.%s", r.ControllerIP(), platform.Hostname())
	entryCmdStr := fmt.Sprintf("echo '%s' >> /etc/hosts", entry)
	for _, n := range c.Nodes {
		if n.Role == "loadbalancer" {
			continue
		}

		hostsContent, _ := docker.Exec(n.Name, "cat /etc/hosts")
		if !strings.Contains(string(hostsContent), entry) {
			fmt.Printf("[%s] adding harbor host entries\n", n.Name)
			_, err := docker.Exec(n.Name, entryCmdStr)
			if err != nil {
				fmt.Printf("[%s] error adding harbor host entry: %s\n", cn, internal.GetCmdStdErr(err))
				os.Exit(1)
			}
			fmt.Printf("[%s] added harbor host entries\n", cn)
		}
	}
}

func (r *Rockpool) AddLagoonRepo(cn string) {
	cmd := helm.Exec(cn, "", "repo", "add", "lagoon", "https://uselagoon.github.io/lagoon-charts/")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("[%s] unable to add lagoon repo: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallLagoonCore() {
	cn := r.ControllerClusterName()
	r.AddLagoonRepo(cn)

	values, err := templates.Render("lagoon-core-values.yml.tmpl", platform.ToMap(), "")
	if err != nil {
		fmt.Printf("[%s] error rendering lagoon-core values template: %s\n", cn, err)
		os.Exit(1)
	}
	fmt.Printf("[%s] using generated lagoon-core values at %s\n", cn, values)

	_, err = helm.InstallOrUpgrade(r.ControllerClusterName(), "lagoon-core",
		"lagoon-core",
		"lagoon/lagoon-core",
		[]string{"--create-namespace", "--wait", "--timeout", "30m0s", "-f", values},
	)
	if err != nil {
		fmt.Printf("[%s] unable to install lagoon-core: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallLagoonRemote(cn string) {
	r.AddLagoonRepo(cn)

	// Get RabbitMQ pass.
	cm := platform.ToMap()
	_, cm["RabbitMQPassword"] = kube.GetSecret(r.ControllerClusterName(),
		"lagoon-core",
		"lagoon-core-broker",
		"RABBITMQ_PASSWORD",
	)

	cm["TargetId"] = fmt.Sprint(internal.GetTargetIdFromCn(cn))

	values, err := templates.Render("lagoon-remote-values.yml.tmpl", cm, cn+"-lagoon-remote-values.yml")
	if err != nil {
		fmt.Printf("[%s] error rendering lagoon-remote values template: %s\n", cn, err)
		os.Exit(1)
	}

	_, err = helm.InstallOrUpgrade(cn, "lagoon", "lagoon-remote",
		"lagoon/lagoon-remote",
		[]string{"--create-namespace", "--wait", "-f", values},
	)
	if err != nil {
		fmt.Printf("[%s] unable to install lagoon-remote: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) RegisterLagoonRemote(cn string) {
	cId := internal.GetTargetIdFromCn(cn)
	rName := platform.Name + fmt.Sprint(cId)
	re := Remote{
		Id:            cId,
		Name:          rName,
		ConsoleUrl:    fmt.Sprintf("https://%s:6443", r.TargetIP(cn)),
		RouterPattern: fmt.Sprintf("${environment}.${project}.%s.%s", rName, platform.Hostname()),
	}
	for _, existingRe := range r.State.Remotes {
		if existingRe.Id == re.Id && existingRe.Name == re.Name {
			fmt.Printf("[%s] Lagoon remote already exists for %s\n", cn, re.Name)
			return
		}
	}
	b64Token, err := kube.Cmd(cn, "lagoon", "get", "secret", "-o=jsonpath='{.items[?(@.metadata.annotations.kubernetes\\.io/service-account\\.name==\"lagoon-remote-kubernetes-build-deploy\")].data.token}'").Output()
	if err != nil {
		fmt.Printf("[%s] error when fetching lagoon remote token: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	token, err := base64.URLEncoding.DecodeString(strings.Trim(string(b64Token), "'"))
	if err != nil {
		fmt.Printf("[%s] error when decoding lagoon remote token: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	r.LagoonApiAddRemote(re, string(token))
}
