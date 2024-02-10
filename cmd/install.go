package cmd

import (
	"log"

	"github.com/salsadigitalauorg/rockpool/pkg/components"
	"github.com/salsadigitalauorg/rockpool/pkg/resolver"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:       "install [component]",
	Aliases:   []string{"i"},
	Short:     "Installs a component on the platform.",
	ValidArgs: append(components.List, "resolver"),
	Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			cmd.Help()
		} else {
			if args[0] == "resolver" {
				resolver.Install()
				return
			}

			log.Print("Installing component: ", args[0])
			components.VerifyRequirements()
			components.Install(args[0])
		}
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
