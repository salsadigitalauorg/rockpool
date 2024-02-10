package cmd

import (
	"strings"

	"github.com/salsadigitalauorg/rockpool/pkg/resolver"
	"github.com/spf13/cobra"
)

var toolCmd = &cobra.Command{
	Use:     "tool [command]",
	Aliases: []string{"t"},
	Short:   "Manage tools on the platform.",
}

var toolInstallCmd = &cobra.Command{
	Use:       "install [tool1,tool2,...]",
	Aliases:   []string{"i"},
	Short:     "Installs a tool on the platform.",
	ValidArgs: []string{"resolver"},
	Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			cmd.Help()
		}
		for _, t := range strings.Split(args[0], ",") {
			switch t {
			case "resolver":
				resolver.Install()
			}
		}
	},
}

func init() {
	toolCmd.AddCommand(toolInstallCmd)
	rootCmd.AddCommand(toolCmd)
}
