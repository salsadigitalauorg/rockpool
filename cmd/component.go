package cmd

import (
	"strings"

	"github.com/salsadigitalauorg/rockpool/pkg/components"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var componentCmd = &cobra.Command{
	Use:     "component [command]",
	Aliases: []string{"co"},
	Short:   "Manage components on the platform.",
}

var componentInstallCmd = &cobra.Command{
	Use:     "install [component1,component2,...]",
	Aliases: []string{"i"},
	Short:   "Installs a component on the platform.",
	Args:    cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			cmd.Help()
			return
		}
		for _, c := range strings.Split(args[0], ",") {
			if !components.IsValid(c) {
				log.Fatalf("invalid argument %q for %q", args[0], cmd.CommandPath())
			}
			log.Print("Installing component: ", c)
			components.VerifyRequirements()
			components.Install(c, false)
		}
	},
}

var componentUpgradeCmd = &cobra.Command{
	Use:     "upgrade [component1,component2,...]",
	Aliases: []string{"u"},
	Short:   "Installs or upgrades a component on the platform.",
	Args:    cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			cmd.Help()
			return
		}
		for _, c := range strings.Split(args[0], ",") {
			if !components.IsValid(c) {
				log.Fatalf("invalid argument %q for %q", args[0], cmd.CommandPath())
			}
			log.Print("Upgrading component: ", c)
			components.VerifyRequirements()
			components.Install(c, true)
		}
	},
}

func init() {
	componentCmd.AddCommand(componentInstallCmd)
	componentCmd.AddCommand(componentUpgradeCmd)
	rootCmd.AddCommand(componentCmd)
}
