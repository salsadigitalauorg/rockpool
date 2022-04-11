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
		fmt.Println("unable to get list of helm releases: ", err)
		os.Exit(1)
	}
	s.HelmReleases = []HelmRelease{}
	_ = json.Unmarshal(out, &s.HelmReleases)
}

func InstallIngressNginx(s *State) {
	err := HelmInstallOrUpgrade(s,
		"ingress-nginx",
		"https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-3.40.0/ingress-nginx-3.40.0.tgz",
		[]string{
			"--create-namespace", "--namespace", "ingress-nginx", "--wait",
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

	err = HelmInstallOrUpgrade(s,
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

func InstallLagoonCore(s *State, c *Config) {
	cmd := exec.Command("helm", "repo", "add", "lagoon", "https://uselagoon.github.io/lagoon-charts/")
	err := internal.RunCmdWithProgress(cmd)
	if err != nil {
		fmt.Println("unable to install lagoon repo: ", err)
		os.Exit(1)
	}

	values, err := internal.RenderTemplate("lagoon-core-values.yml.tmpl", c.RenderedTemplatesPath, c)
	if err != nil {
		fmt.Println("error rendering harbor values template: ", err)
		os.Exit(1)
	}
	fmt.Println("using generated harbor values at ", values)

	err = HelmInstallOrUpgrade(s,
		"lagoon-core",
		"lagoon/lagoon-core",
		[]string{
			"--create-namespace", "--namespace", "lagoon-core",
			"-f", values,
		},
	)
	if err != nil {
		fmt.Println("unable to install harbor: ", err)
		os.Exit(1)
	}
}
