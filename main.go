package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"runtime"

	"github.com/spf13/pflag"
	"github.com/yusufhm/rockpool/pkg/rockpool"
)

var r rockpool.Rockpool
var down bool
var stop bool

func main() {
	r = rockpool.Rockpool{
		State: rockpool.State{
			Clusters:     rockpool.ClusterList{},
			BinaryPaths:  map[string]string{},
			HelmReleases: map[string][]rockpool.HelmRelease{},
			Kubeconfig:   map[string]string{},
		},
		Config: rockpool.Config{},
	}
	parseFlags()
	r.Config.Arch = runtime.GOARCH

	r.VerifyReqs()
	r.FetchClusters()
	fmt.Println()

	if stop {
		r.Stop()
		os.Exit(0)
	}

	if down {
		r.Down()
		os.Exit(0)
	}

	r.LagoonController()
	r.LagoonTarget()
}

func parseFlags() {
	pflag.ErrHelp = errors.New("rockpool: help requested")

	pflag.Usage = func() {
		fmt.Fprint(os.Stderr, "Rockpool\n\nEasily create local Lagoon instances.\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  %s \n\nFlags:\n", os.Args[0])
		pflag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
	}

	displayUsage := pflag.BoolP("help", "h", false, "Displays usage information")
	pflag.StringVarP(&r.Config.ClusterName, "cluster-name", "n", "rockpool", "The name of the cluster")
	pflag.StringVarP(&r.Config.LagoonBaseUrl, "lagoon-base-url", "l", "lagoon.rockpool.k3d.local", `The base Lagoon url of the cluster;
all Lagoon services will be created as subdomains of this url, e.g,
ui.lagoon.rockpool.k3d.local, harbor.lagoon.rockpool.k3d.local`)
	pflag.StringVar(&r.Config.HarborPass, "harbor-password", "pass", "The Harbor password")

	defaultRenderedPath := path.Join(os.TempDir(), "rockpool", "rendered")
	pflag.StringVar(&r.Config.RenderedTemplatesPath, "rendered-template-path", defaultRenderedPath, "The directory where rendered template files are placed")
	pflag.StringSliceVar(&r.Config.UpgradeComponents, "upgrade-components", []string{}, "A list of components to upgrade, e.g, ingress-nginx,harbor")
	pflag.BoolVar(&down, "down", false, "Stops the cluster and deletes it")
	pflag.BoolVar(&stop, "stop", false, "Stops the cluster")

	pflag.Parse()

	if *displayUsage {
		pflag.Usage()
		os.Exit(0)
	}
}
