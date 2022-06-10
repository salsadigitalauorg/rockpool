package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/yusufhm/rockpool/pkg/rockpool"
)

var r = rockpool.Rockpool{
	State: rockpool.State{
		Spinner:      *spinner.New(spinner.CharSets[14], 100*time.Millisecond),
		Clusters:     rockpool.ClusterList{},
		BinaryPaths:  sync.Map{},
		HelmReleases: sync.Map{},
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
	Use:   "up [name...]",
	Short: "Create and/or start the clusters",
	Long: `up is for creating or starting all the clusters, or the ones
specified in the arguments, e.g, 'rockpool up controller target-1'`,
	Run: func(cmd *cobra.Command, args []string) {
		r.VerifyReqs(true)
		r.FetchClusters()
		r.Up(fullClusterNamesFromArgs(args))
	},
}

var startCmd = &cobra.Command{
	Use:   "start [name...]",
	Short: "Start the clusters",
	Long: `start is for starting all the clusters, or the ones
specified in the arguments, e.g, 'rockpool start controller target-1'`,
	Run: func(cmd *cobra.Command, args []string) {
		r.VerifyReqs(false)
		r.FetchClusters()
		r.Start(fullClusterNamesFromArgs(args))
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop [name...]",
	Short: "Stop the clusters",
	Long: `stop is for stopping all the clusters, or the ones
specified in the arguments, e.g, 'rockpool stop controller target-1'`,
	Run: func(cmd *cobra.Command, args []string) {
		r.VerifyReqs(false)
		r.FetchClusters()
		r.Stop(fullClusterNamesFromArgs(args))
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart [name...]",
	Short: "Restart the clusters",
	Long: `restart is for stopping and starting all the clusters, or the ones
specified in the arguments, e.g, 'rockpool restart controller target-1'`,
	Run: func(cmd *cobra.Command, args []string) {
		r.VerifyReqs(false)
		r.FetchClusters()
		r.Stop(fullClusterNamesFromArgs(args))
		r.Start(fullClusterNamesFromArgs(args))
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "View the status of the clusters",
	Run: func(cmd *cobra.Command, args []string) {
		r.Status()
	},
}

var downCmd = &cobra.Command{
	Use:   "down [name...]",
	Short: "Stop the clusters and delete them",
	Long: `down is for stopping and deleting all the clusters, or the ones
specified in the arguments, e.g, 'rockpool down controller target-1'`,
	Run: func(cmd *cobra.Command, args []string) {
		r.VerifyReqs(false)
		r.FetchClusters()
		r.Down(fullClusterNamesFromArgs(args))
	},
}

func fullClusterNamesFromArgs(argClusters []string) []string {
	clusters := []string{}
	for _, c := range argClusters {
		clusters = append(clusters, r.Config.Name+"-"+c)
	}
	return clusters
}

func init() {
	determineConfigDir()
	r.Initialise()

	rootCmd.PersistentFlags().StringVarP(&r.Config.Name, "name", "n", "rockpool", "The name of the platform")

	upCmd.Flags().IntVarP(&r.Config.NumTargets, "targets", "t", 1, "Number of targets (lagoon remotes) to create")
	upCmd.Flags().StringVarP(&r.Config.Domain, "domain", "d", "k3d.local",
		`The base domain of the platform; ancillary services will be created as its
subdomains using the provided 'name', e.g, rockpool.k3d.local, lagoon.rockpool.k3d.local
`)
	upCmd.Flags().StringSliceVar(&r.Config.UpgradeComponents, "upgrade-components", []string{},
		"A list of components to upgrade, e.g, all or ingress-nginx,harbor")
	upCmd.Flags().StringVarP(&r.Config.LagoonSshKey, "ssh-key", "k", "",
		`The ssh key to add to the lagoonadmin user. If empty, rockpool tries
to use ~/.ssh/id_ed25519.pub first, then ~/.ssh/id_rsa.pub.
`)

	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(statusCmd)
}

func determineConfigDir() {
	var err error
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	r.Config.ConfigDir = filepath.Join(userHomeDir, ".rockpool")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
