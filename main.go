package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/pflag"
	r "github.com/yusufhm/rockpool/pkg/rockpool"
)

var state r.State
var config r.Config

func main() {
	config = r.Config{}
	parseFlags()

	state = r.State{}
	r.VerifyReqs(&state)
	fmt.Println()

	r.CreateCluster(&state, config.ClusterName)
	fmt.Println()
	r.GetClusterKubeConfigPath(&state, config.ClusterName)

	r.ClusterVersion(&state)
	fmt.Println()
	r.HelmList(&state)
	r.InstallIngressNginx(&state)

	r.InstallHarbor(&state, &config)
	r.InstallLagoonCore(&state, &config)
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
	pflag.StringVar(&config.HarborPass, "harbor-password", "pass", "The Harbor password.")

	pflag.Parse()

	if *displayUsage {
		pflag.Usage()
		os.Exit(0)
	}
}
