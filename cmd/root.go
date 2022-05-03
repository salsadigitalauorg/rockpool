package cmd

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/yusufhm/rockpool/pkg/rockpool"
)

// var configDir string
var r = rockpool.Rockpool{
	State: rockpool.State{
		Spinner:      *spinner.New(spinner.CharSets[14], 100*time.Millisecond),
		Clusters:     rockpool.ClusterList{},
		BinaryPaths:  map[string]string{},
		HelmReleases: map[string][]rockpool.HelmRelease{},
		Kubeconfig:   map[string]string{},
	},
	Config: rockpool.Config{},
}

var rootCmd = &cobra.Command{
	Use:   "rockpool [command]",
	Short: "Easily create local Lagoon instances.",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Usage()
	},
}

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Create and/or start the clusters",
	Run: func(cmd *cobra.Command, args []string) {
		r.Up()
	},
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the clusters",
	Run: func(cmd *cobra.Command, args []string) {
		r.Start()
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the clusters",
	Run: func(cmd *cobra.Command, args []string) {
		r.Stop()
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the clusters",
	Run: func(cmd *cobra.Command, args []string) {
		r.Stop()
		r.Up()
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "View the status of the clusters",
	Run: func(cmd *cobra.Command, args []string) {
	},
}

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the clusters and delete them",
	Run: func(cmd *cobra.Command, args []string) {
		r.Down()
	},
}

func init() {
	// determineConfigDir()
	r.Spinner.Color("red", "bold")
	r.Config.Arch = runtime.GOARCH

	upCmd.Flags().StringVarP(&r.Config.ClusterName, "cluster-name", "n", "rockpool", "The name of the cluster")
	upCmd.Flags().StringVarP(&r.Config.Hostname, "url", "u", "rockpool.k3d.local",
		`The base url of rockpool; ancillary services will be created
as subdomains of this url, e.g, gitlab.rockpool.k3d.local
`)
	upCmd.Flags().StringVarP(&r.Config.LagoonBaseUrl, "lagoon-base-url", "l", "lagoon.rockpool.k3d.local",
		`The base Lagoon url of the cluster;
all Lagoon services will be created as subdomains of this url, e.g,
ui.lagoon.rockpool.k3d.local, harbor.lagoon.rockpool.k3d.local
`)
	upCmd.Flags().StringVar(&r.Config.HarborPass, "harbor-password", "pass", "The Harbor password")

	defaultRenderedPath := path.Join(os.TempDir(), "rockpool", "rendered")
	upCmd.Flags().StringVar(&r.Config.RenderedTemplatesPath, "rendered-template-path", defaultRenderedPath,
		`The directory where rendered template files are placed
`)
	upCmd.Flags().StringSliceVar(&r.Config.UpgradeComponents, "upgrade-components", []string{},
		"A list of components to upgrade, e.g, ingress-nginx,harbor")

	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(statusCmd)
}

// func determineConfigDir() {
// 	var err error
// 	userConfigDir, err := os.UserConfigDir()
// 	if err != nil {
// 		fmt.Fprintln(os.Stderr, "unable to get user config dir: ", err)
// 		os.Exit(1)
// 	}
// 	configDir = filepath.Join(userConfigDir, "rockpool")
// }

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
