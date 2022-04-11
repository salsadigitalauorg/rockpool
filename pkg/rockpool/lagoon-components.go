package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"text/template"

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
	err := helmInstallOrUpgrade(s,
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

	t := template.Must(template.ParseFiles("templates/harbor-values.yml.tmpl"))

	f, err := os.Create("rendered/harbor-values.yml")
	if err != nil {
		fmt.Println("error creating harbor values file: ", err)
		return
	}

	config := map[string]string{
		"lagoonBaseUrl": c.LagoonBaseUrl,
		"password":      c.HarborPass,
	}
	err = t.Execute(f, config)
	if err != nil {
		fmt.Println("error rendering harbor values template: ", err)
		return
	}
	f.Close()

	err = helmInstallOrUpgrade(s,
		"harbor",
		"harbor/harbor",
		[]string{
			"--create-namespace", "--namespace", "harbor", "--wait",
			"-f", "rendered/harbor-values.yml", "--version=1.5.6",
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

	t := template.Must(template.ParseFiles("templates/lagoon-core-values.yml.tmpl"))

	f, err := os.Create("rendered/lagoon-core-values.yml")
	if err != nil {
		fmt.Println("error creating lagoon core values file: ", err)
		return
	}

	config := map[string]string{
		"arch":          runtime.GOARCH,
		"lagoonBaseUrl": c.LagoonBaseUrl,
	}
	err = t.Execute(f, config)
	if err != nil {
		fmt.Println("error rendering lagoon core values template: ", err)
		return
	}
	f.Close()

	err = helmInstallOrUpgrade(s,
		"lagoon-core",
		"lagoon/lagoon-core",
		[]string{
			"--create-namespace", "--namespace", "lagoon-core",
			"-f", "rendered/lagoon-core-values.yml",
		},
	)
	if err != nil {
		fmt.Println("unable to install harbor: ", err)
		os.Exit(1)
	}
}
