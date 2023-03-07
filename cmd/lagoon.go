package cmd

import (
	"fmt"

	"github.com/salsadigitalauorg/rockpool/pkg/lagoon"
	"github.com/spf13/cobra"
)

var lagoonCmd = &cobra.Command{
	Use:   "lagoon [command]",
	Short: "Execute lagoon operations.",
}

var lagoonAdminTokenCmd = &cobra.Command{
	Use:   "admin-token",
	Short: "Fetch an admin token for the Lagoon API.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(lagoon.FetchApiAdminToken())
	},
}

func init() {
	lagoonCmd.AddCommand(lagoonAdminTokenCmd)
	rootCmd.AddCommand(lagoonCmd)
}
