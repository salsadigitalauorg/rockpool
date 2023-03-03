package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/salsadigitalauorg/rockpool/pkg/helm"
	"github.com/salsadigitalauorg/rockpool/pkg/lagoon"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	r "github.com/salsadigitalauorg/rockpool/pkg/rockpool"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Version information.
var (
	Version string
	Commit  string
)

var logLevel string
var debug bool
var trace bool

var rootCmd = &cobra.Command{
	Use:   "rockpool [command]",
	Short: "Easily create local Lagoon instances.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if debug {
			logLevel = "debug"
		}
		if trace {
			logLevel = "trace"
		}
		if logrusLevel, err := log.ParseLevel(logLevel); err != nil {
			panic(err)
		} else {
			log.SetLevel(logrusLevel)
		}
		// Do not initialise when just running the root command.
		if cmd.Use == "rockpool [command]" {
			return
		}
		r.Initialise()
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Usage()
	},
}

var versionCmd = &cobra.Command{
	Use:              "version",
	Short:            "Displays the application version",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Version:", Version)
		fmt.Println("Commit:", Commit)
	},
}

var upCmd = &cobra.Command{
	Use:   "up [name...]",
	Short: "Create and/or start the clusters",
	Long: `up is for creating or starting all the clusters, or the ones
specified in the arguments, e.g, 'rockpool up controller target-1'`,
	Run: func(cmd *cobra.Command, args []string) {
		r.Up(fullClusterNamesFromArgs(args))
	},
}

var startCmd = &cobra.Command{
	Use:   "start [name...]",
	Short: "Start the clusters",
	Long: `start is for starting all the clusters, or the ones
specified in the arguments, e.g, 'rockpool start controller target-1'`,
	Run: func(cmd *cobra.Command, args []string) {
		r.Start(fullClusterNamesFromArgs(args))
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop [name...]",
	Short: "Stop the clusters",
	Long: `stop is for stopping all the clusters, or the ones
specified in the arguments, e.g, 'rockpool stop controller target-1'`,
	Run: func(cmd *cobra.Command, args []string) {
		r.Stop(fullClusterNamesFromArgs(args))
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart [name...]",
	Short: "Restart the clusters",
	Long: `restart is for stopping and starting all the clusters, or the ones
specified in the arguments, e.g, 'rockpool restart controller target-1'`,
	Run: func(cmd *cobra.Command, args []string) {
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
		r.Down(fullClusterNamesFromArgs(args))
	},
}

func fullClusterNamesFromArgs(argClusters []string) []string {
	clusters := []string{}
	for _, c := range argClusters {
		clusters = append(clusters, platform.Name+"-"+c)
	}
	return clusters
}

func init() {
	determineConfigDir()

	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "sets the logging level")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enables debug logging (similar to `--log-level debug`)")
	rootCmd.PersistentFlags().BoolVar(&trace, "trace", false, "enables trace logging (similar to `--log-level trace`)")

	rootCmd.PersistentFlags().StringVarP(&platform.Name, "name", "n", "rockpool", "The name of the platform")

	upCmd.Flags().IntVarP(&platform.NumTargets, "targets", "t", 1, "Number of targets (lagoon remotes) to create")
	upCmd.Flags().StringVarP(&platform.Domain, "domain", "d", "k3d.local",
		`The base domain of the platform; ancillary services will be created as its
subdomains using the provided 'name', e.g, rockpool.k3d.local, lagoon.rockpool.k3d.local
`)

	upCmd.Flags().StringVarP(&lagoon.Version, "lagoon-version", "l", "v2.7.1", "The version of Lagoon to install")
	upCmd.Flags().StringSliceVar(&helm.UpgradeComponents, "upgrade-components", []string{},
		"A list of components to upgrade, e.g, all or ingress-nginx,harbor")
	upCmd.Flags().StringVarP(&platform.LagoonSshKey, "ssh-key", "k", "",
		`The ssh key to add to the lagoonadmin user. If empty, rockpool tries
to use ~/.ssh/id_ed25519.pub first, then ~/.ssh/id_rsa.pub.
`)

	rootCmd.AddCommand(versionCmd)
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
	platform.ConfigDir = filepath.Join(userHomeDir, ".rockpool")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
