package cmd

import (
	"github.com/spf13/cobra"

	"github.com/salsadigitalauorg/rockpool/pkg/clusters"
)

var clustersCmd = &cobra.Command{
	Use:     "clusters [command]",
	Aliases: []string{"c"},
	Short:   "Execute operations on the clusters.",
}

var clustersStatusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st"},
	Short:   "Fetch the status of the clusters.",
	Run: func(cmd *cobra.Command, args []string) {
		clusters.VerifyRequirements()
		clusters.Status()
	},
}

var clustersCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"c"},
	Short:   "Create the clusters.",
	Run: func(cmd *cobra.Command, args []string) {
		clusters.VerifyRequirements()
		clusters.Ensure()
	},
}

var clustersStopCmd = &cobra.Command{
	Use:     "stop",
	Aliases: []string{"s"},
	Short:   "Stop the clusters.",
	Run: func(cmd *cobra.Command, args []string) {
		clusters.VerifyRequirements()
		clusters.Stop()
	},
}

var clustersDeleteCmd = &cobra.Command{
	Use:     "delete",
	Aliases: []string{"s"},
	Short:   "Delete the clusters.",
	Run: func(cmd *cobra.Command, args []string) {
		clusters.VerifyRequirements()
		clusters.Stop()
		clusters.Delete()
	},
}

func init() {
	clustersCmd.AddCommand(clustersStatusCmd)
	clustersCmd.AddCommand(clustersCreateCmd)
	clustersCmd.AddCommand(clustersStopCmd)
	clustersCmd.AddCommand(clustersDeleteCmd)
	rootCmd.AddCommand(clustersCmd)
}
