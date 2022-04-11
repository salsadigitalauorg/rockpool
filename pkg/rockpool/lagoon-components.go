package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/yusufhm/rockpool/internal"
)

func HelmList(s *State) {
	cmd := exec.Command("helm", "list", "--all-namespaces", "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		fmt.Println(string(out))
		fmt.Println("unable to get list of helm releases: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	s.HelmReleases = []HelmRelease{}
	_ = json.Unmarshal(out, &s.HelmReleases)
}

func InstallIngressNginx(s *State, c *Config) {
	err := HelmInstallOrUpgrade(s, c,
		"ingress-nginx",
		"https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-3.40.0/ingress-nginx-3.40.0.tgz",
		[]string{
			"--create-namespace", "--namespace", "ingress-nginx", "--wait",
			"--set", "controller.config.ssl-redirect=false",
		},
	)
	if err != nil {
		fmt.Println("unable to install ingress-nginx: ", err)
		os.Exit(1)
	}
}

func InstallHarbor(s *State, c *Config) {
	cmd := exec.Command("helm", "repo", "add", "harbor", "https://helm.goharbor.io")
	err := internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Println("unable to install harbor repo: ", err)
		os.Exit(1)
	}

	values, err := internal.RenderTemplate("harbor-values.yml.tmpl", c.RenderedTemplatesPath, c)
	if err != nil {
		fmt.Println("error rendering harbor values template: ", err)
		os.Exit(1)
	}
	fmt.Println("using generated harbor values at ", values)

	err = HelmInstallOrUpgrade(s, c,
		"harbor",
		"harbor/harbor",
		[]string{
			"--create-namespace", "--namespace", "harbor", "--wait",
			"-f", values, "--version=1.5.6",
		},
	)
	if err != nil {
		fmt.Println("unable to install harbor: ", err)
		os.Exit(1)
	}
}

func AddLagoonRepo(s *State) {
	cmd := exec.Command(
		"helm", "--kubeconfig", s.Kubeconfig, "repo", "add",
		"lagoon", "https://uselagoon.github.io/lagoon-charts/",
	)
	err := internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Println("unable to install lagoon repo: ", err)
		os.Exit(1)
	}
}

func InstallLagoonCore(s *State, c *Config) {
	AddLagoonRepo(s)

	values, err := internal.RenderTemplate("lagoon-core-values.yml.tmpl", c.RenderedTemplatesPath, c)
	if err != nil {
		fmt.Println("error rendering lagoon-core values template: ", err)
		os.Exit(1)
	}
	fmt.Println("using generated lagoon-core values at ", values)

	err = HelmInstallOrUpgrade(s, c,
		"lagoon-core",
		"lagoon/lagoon-core",
		[]string{
			"--create-namespace", "--namespace", "lagoon-core",
			"-f", values,
		},
	)
	if err != nil {
		fmt.Println("unable to install lagoon-core: ", err)
		os.Exit(1)
	}
}

func InstallLagoonRemote(s *State, c *Config) {
	AddLagoonRepo(s)

	// Get RabbitMQ pass.
	cm := c.ToMap()
	cm["RabbitMQPassword"] = KubeGetSecret(s, "lagoon-core", "lagoon-core-broker", "RABBITMQ_PASSWORD")

	values, err := internal.RenderTemplate("lagoon-remote-values.yml.tmpl", c.RenderedTemplatesPath, cm)
	if err != nil {
		fmt.Println("error rendering lagoon-remote values template: ", err)
		os.Exit(1)
	}
	fmt.Println("using generated lagoon-remote values at ", values)

	err = HelmInstallOrUpgrade(s, c,
		"lagoon-remote",
		"lagoon/lagoon-remote",
		[]string{
			"--create-namespace", "--namespace", "lagoon",
			"-f", values,
		},
	)
	if err != nil {
		fmt.Println("unable to install lagoon-remote: ", err)
		os.Exit(1)
	}
}
