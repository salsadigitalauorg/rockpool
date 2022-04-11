package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"runtime"

	"github.com/spf13/pflag"
	r "github.com/yusufhm/rockpool/pkg/rockpool"
)

var state r.State
var config r.Config

func main() {
	config = r.Config{}
	parseFlags()
	config.Arch = runtime.GOARCH

	state = r.State{}
	r.VerifyReqs(&state, &config)
	fmt.Println()

	r.CreateCluster(&state, config.ClusterName)
	fmt.Println()
	r.GetClusterKubeConfigPath(&state, config.ClusterName)

	// Install mailhog.
	r.KubeApply(&state, &config, "mailhog.yml.tmpl", true)

	r.ClusterVersion(&state)
	fmt.Println()
	r.HelmList(&state)
	r.InstallIngressNginx(&state, &config)

	r.InstallHarbor(&state, &config)
	r.InstallLagoonCore(&state, &config)

	r.ConfigureKeycloak(&state)

	r.InstallLagoonRemote(&state, &config)
}

func parseFlags() {
	pflag.ErrHelp = errors.New("rockpool: help requested")

	pflag.Usage = func() {
		fmt.Fprint(os.Stderr, "Rockpool\n\nEasily create local Lagoon instances.\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  %s [dir]\n\nFlags:\n", os.Args[0])
		pflag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
	}

	displayUsage := pflag.BoolP("help", "h", false, "Displays usage information")
	pflag.StringVarP(&config.ClusterName, "cluster-name", "n", "rockpool", "The name of the cluster")
	pflag.StringVarP(&config.LagoonBaseUrl, "lagoon-base-url", "l", "lagoon.rockpool.k3d.local", `The base Lagoon url of the cluster;
all Lagoon services will be created as subdomains of this url, e.g,
ui.lagoon.rockpool.k3d.local, harbor.lagoon.rockpool.k3d.local`)
	pflag.StringVar(&config.HarborPass, "harbor-password", "pass", "The Harbor password")

	defaultRenderedPath := path.Join(os.TempDir(), "rockpool", "rendered")
	pflag.StringVar(&config.RenderedTemplatesPath, "rendered-template-path", defaultRenderedPath, "The directory where rendered template files are placed")
	pflag.StringSliceVar(&config.UpgradeComponents, "upgrade-components", []string{}, "A list of components to upgrade, e.g, ingress-nginx,harbor")

	pflag.Parse()

	if *displayUsage {
		pflag.Usage()
		os.Exit(0)
	}
}
