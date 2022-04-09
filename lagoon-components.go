package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"text/template"
)

func helmList() {
	cmd := exec.Command("helm", "list", "--all-namespaces", "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		fmt.Println(string(out))
		fmt.Println("unable to get list of helm releases: ", err)
		os.Exit(1)
	}
	helmReleases = []HelmRelease{}
	_ = json.Unmarshal(out, &helmReleases)
}

func installIngressNginx() {
	err := helmInstallOrUpgrade(
		"ingress-nginx",
		"https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-3.40.0/ingress-nginx-3.40.0.tgz",
		[]string{
			"--create-namespace",
			"--namespace",
			"ingress-nginx",
		},
	)
	if err != nil {
		fmt.Println("unable to install ingress-nginx: ", err)
		os.Exit(1)
	}
}

func installHarbor(host string, password string) {
	cmd := exec.Command("helm", "repo", "add", "harbor", "https://helm.goharbor.io")
	err := runCmdWithProgress(cmd)
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
		"host":     host,
		"password": password,
	}
	err = t.Execute(f, config)
	if err != nil {
		fmt.Println("error rendering harbor values template: ", err)
		return
	}
	f.Close()
}
