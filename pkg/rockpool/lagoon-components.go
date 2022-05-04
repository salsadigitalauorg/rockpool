package rockpool

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/yusufhm/rockpool/internal"
)

func (r *Rockpool) InstallIngressNginx() {
	_, err := r.HelmInstallOrUpgrade(r.ControllerClusterName(),
		"ingress-nginx", "ingress-nginx",
		"https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-3.40.0/ingress-nginx-3.40.0.tgz",
		[]string{
			"--create-namespace", "--wait",
			"--set", "controller.config.ssl-redirect=false",
		},
	)
	if err != nil {
		fmt.Println("unable to install ingress-nginx: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallHarbor() {
	cmd := r.Helm(
		r.ControllerClusterName(), "",
		"repo", "add", "harbor", "https://helm.goharbor.io",
	)
	err := cmd.Run()
	if err != nil {
		fmt.Println("unable to add harbor repo: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}

	values, err := internal.RenderTemplate("harbor-values.yml.tmpl", r.Config.RenderedTemplatesPath, r.Config, "")
	if err != nil {
		fmt.Println("error rendering harbor values template: ", err)
		os.Exit(1)
	}
	fmt.Println("using generated harbor values at ", values)

	_, err = r.HelmInstallOrUpgrade(r.ControllerClusterName(),
		"harbor", "harbor", "harbor/harbor",
		[]string{
			"--create-namespace", "--wait",
			"-f", values, "--version=1.5.6",
		},
	)
	if err != nil {
		fmt.Println("unable to install harbor: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) FetchHarborCerts() (string, string) {
	certBytes, _ := r.KubeGetSecret(r.ControllerClusterName(), "harbor", "harbor-harbor-ingress", "")
	certData := struct {
		Data map[string]string `json:"data"`
	}{}
	json.Unmarshal(certBytes, &certData)

	secretManifest, err := internal.RenderTemplate("harbor-cert.yml.tmpl", r.Config.RenderedTemplatesPath, certData, "")
	if err != nil {
		fmt.Println("error rendering harbor cert template: ", err)
		os.Exit(1)
	}
	fmt.Println("generated harbor cert at", secretManifest)

	cacrt := certData.Data["ca.crt"]
	decoded, err := base64.URLEncoding.DecodeString(cacrt)
	if err != nil {
		fmt.Printf("error when decoding ca.crt: %#v", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	caCrtFile, err := internal.RenderTemplate("harbor-ca.crt.tmpl", r.Config.RenderedTemplatesPath, string(decoded), "")
	if err != nil {
		fmt.Println("error rendering harbor ca.crt template: ", err)
		os.Exit(1)
	}
	fmt.Println("generated harbor ca.crt at", caCrtFile)

	return secretManifest, caCrtFile
}

func (r *Rockpool) InstallHarborCerts(cn string) {
	if cn == "rockpool-controller" {
		return
	}

	exists, c := r.Clusters.ClusterExists(cn)
	if !exists {
		fmt.Println("cluster", cn, "does not exist")
		return
	}

	secretManifest, caCrtFile := r.FetchHarborCerts()
	if _, err := r.KubeApply(cn, "lagoon", secretManifest, true); err != nil {
		fmt.Printf("error creating ca.crt in %s: %s\n", cn, err)
		os.Exit(1)
	}

	// Add the cert to the nodes.
	clusterUpdated := false
	for _, n := range c.Nodes {
		if n.Role == "loadbalancer" {
			continue
		}

		caCrtFileOut, _ := r.DockerExec(n.Name, "ls /etc/ssl/certs/harbor-cert.crt")
		if strings.Trim(string(caCrtFileOut), "\n") == "/etc/ssl/certs/harbor-cert.crt" {
			continue
		}

		// Add harbor's ca.crt to the target.
		destCaCrt := fmt.Sprintf("%s:/etc/ssl/certs/harbor-cert.crt", n.Name)
		_, err := r.DockerCp(caCrtFile, destCaCrt)
		if err != nil {
			fmt.Printf("error copying ca.crt to %s: %s\n", c.Name, internal.GetCmdStdErr(err))
			os.Exit(1)
		}
		clusterUpdated = true
	}

	if clusterUpdated {
		r.RestartCluster(c.Name)
	}

	// Patch lagoon-remote-lagoon-build-deploy to add the cert secret.
	patchFile, err := internal.RenderTemplate("patch-lagoon-remote-lagoon-build-deploy.yaml", r.Config.RenderedTemplatesPath, nil, "")
	if err != nil {
		fmt.Println("error rendering the build deploy patch file: ", err)
		os.Exit(1)
	}
	_, err = r.KubePatch(cn, "lagoon", "deployment", "lagoon-remote-lagoon-build-deploy", patchFile)
	if err != nil {
		fmt.Println("error patching the lagoon-build-deploy deployment: ", err)
		os.Exit(1)
	}
}

// AddHarborHostEntries adds host entries to the target nodes.
func (r *Rockpool) AddHarborHostEntries(cn string) {
	if cn == "rockpool-controller" {
		return
	}

	exists, c := r.Clusters.ClusterExists(cn)
	if !exists {
		fmt.Println("cluster", cn, "does not exist")
		return
	}

	entry := fmt.Sprintf("%s\tharbor.%s", r.ControllerIP(), r.Config.LagoonBaseUrl)
	entryCmdStr := fmt.Sprintf("echo '%s' >> /etc/hosts", entry)
	for _, n := range c.Nodes {
		if n.Role == "loadbalancer" {
			continue
		}

		hostsContent, _ := r.DockerExec(n.Name, "cat /etc/hosts")
		if !strings.Contains(string(hostsContent), entry) {
			fmt.Printf("adding host entries to %s...\n", n.Name)
			_, err := r.DockerExec(n.Name, entryCmdStr)
			if err != nil {
				fmt.Printf("error adding host entry in %s: %s\n", c.Name, internal.GetCmdStdErr(err))
				os.Exit(1)
			}
			fmt.Println("added host entries to", n.Name)
		}
	}
}

func (r *Rockpool) AddLagoonRepo(cn string) {
	cmd := r.Helm(
		cn, "", "repo", "add", "lagoon",
		"https://uselagoon.github.io/lagoon-charts/",
	)
	err := cmd.Run()
	if err != nil {
		fmt.Println("unable to add lagoon repo: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallLagoonCore() {
	r.AddLagoonRepo(r.ControllerClusterName())

	values, err := internal.RenderTemplate(
		"lagoon-core-values.yml.tmpl",
		r.Config.RenderedTemplatesPath, r.Config, "",
	)
	if err != nil {
		fmt.Println("error rendering lagoon-core values template: ", err)
		os.Exit(1)
	}
	fmt.Println("using generated lagoon-core values at ", values)

	_, err = r.HelmInstallOrUpgrade(r.ControllerClusterName(), "lagoon-core",
		"lagoon-core",
		"lagoon/lagoon-core",
		[]string{"--create-namespace", "--wait", "--timeout", "30m0s", "-f", values},
	)
	if err != nil {
		fmt.Println("unable to install lagoon-core: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) InstallLagoonRemote(cn string) {
	r.AddLagoonRepo(cn)

	// Get RabbitMQ pass.
	cm := r.Config.ToMap()
	_, cm["RabbitMQPassword"] = r.KubeGetSecret(r.ControllerClusterName(),
		"lagoon-core",
		"lagoon-core-broker",
		"RABBITMQ_PASSWORD",
	)

	cm["TargetId"] = fmt.Sprint(internal.GetTargetIdFromCn(cn))

	values, err := internal.RenderTemplate(
		"lagoon-remote-values.yml.tmpl",
		r.Config.RenderedTemplatesPath, cm,
		cn+"-lagoon-remote-values.yml",
	)
	if err != nil {
		fmt.Println("error rendering lagoon-remote values template: ", err)
		os.Exit(1)
	}

	_, err = r.HelmInstallOrUpgrade(cn, "lagoon", "lagoon-remote",
		"lagoon/lagoon-remote",
		[]string{"--create-namespace", "--wait", "-f", values},
	)
	if err != nil {
		fmt.Println("unable to install lagoon-remote: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
}

func (r *Rockpool) RegisterLagoonRemote(cn string) {
	cId := internal.GetTargetIdFromCn(cn)
	rName := "rockpool" + fmt.Sprint(cId)
	re := Remote{
		Id:            cId,
		Name:          rName,
		ConsoleUrl:    fmt.Sprintf("https://%s:6443", r.TargetIP(cn)),
		RouterPattern: fmt.Sprintf("${environment}.${project}.%s.%s", rName, r.Hostname),
	}
	for _, existingRe := range r.State.Remotes {
		if existingRe.Id == re.Id && existingRe.Name == re.Name {
			fmt.Println("Lagoon remote already exists for", re.Name)
			return
		}
	}
	b64Token, err := r.KubeCtl(cn, "lagoon", "get", "secret", "-o=jsonpath='{.items[?(@.metadata.annotations.kubernetes\\.io/service-account\\.name==\"lagoon-remote-kubernetes-build-deploy\")].data.token}'").Output()
	if err != nil {
		fmt.Println("error when fetching lagoon remote token: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	token, err := base64.URLEncoding.DecodeString(strings.Trim(string(b64Token), "'"))
	if err != nil {
		fmt.Printf("error when decoding lagoon remote token for %s: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	r.LagoonApiAddRemote(re, string(token))
}
