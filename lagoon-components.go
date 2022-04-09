package main

import (
	"fmt"
	"os"
)

func installIngressNginx() {
	err := helmInstallOrUpgrade(
		"ingress-nginx",
		"https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-3.40.0/ingress-nginx-3.40.0.tgz",
		true,
		[]string{
			"--create-namespace",
			"--namespace",
			"ingress-nginx",
		},
	)
	if err != nil {
		fmt.Printf("unable to install ingress-nginx: %s", err)
		os.Exit(1)
	}
}
