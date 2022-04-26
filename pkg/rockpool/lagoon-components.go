package rockpool

import (
	"fmt"
	"os"

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

	values, err := internal.RenderTemplate("harbor-values.yml.tmpl", r.Config.RenderedTemplatesPath, r.Config)
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
		r.Config.RenderedTemplatesPath, r.Config,
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
	cm["RabbitMQPassword"] = r.KubeGetSecret(r.ControllerClusterName(),
		"lagoon-core",
		"lagoon-core-broker",
		"RABBITMQ_PASSWORD",
	)

	values, err := internal.RenderTemplate(
		"lagoon-remote-values.yml.tmpl",
		r.Config.RenderedTemplatesPath, cm,
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
