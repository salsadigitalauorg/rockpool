package cmd

import (
	"fmt"

	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/k3d"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var kubeClusterControllerOnly bool
var kubeClusterTargetOnly int
var clusterNames []string

var kubeCmd = &cobra.Command{
	Use:   "kube [command]",
	Short: "Execute kubernetes operations.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		k3d.ClusterFetch()
		if kubeClusterControllerOnly || (!kubeClusterControllerOnly && kubeClusterTargetOnly == 0) {
			clusterNames = append(clusterNames, platform.ControllerClusterName())
		}
		if kubeClusterControllerOnly {
			return
		}
		if len(k3d.Clusters) > 1 {
			for _, c := range k3d.Clusters {
				if c.Name == platform.ControllerClusterName() {
					continue
				}
				if kubeClusterTargetOnly > 0 &&
					c.Name != platform.TargetClusterName(kubeClusterTargetOnly) {
					continue
				}
				clusterNames = append(clusterNames, c.Name)
			}
		}
	},
}

var kubeConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Outputs the kubeconfig path for the cluster(s)",
	PreRun: func(cmd *cobra.Command, args []string) {
		if len(clusterNames) == 0 {
			log.Fatal("no cluster found")
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		for _, cn := range clusterNames {
			fmt.Println(kube.KubeconfigPath(cn))
		}
	},
}

var kubeCtlCmd = &cobra.Command{
	Use:     "kubectl",
	Aliases: []string{"k"},
	Short:   "Runs kubectl commands with the specified cluster",
	PreRun: func(cmd *cobra.Command, args []string) {
		if len(clusterNames) == 0 {
			log.Info("no cluster specifed, setting to controller")
			clusterNames = append(clusterNames, "controller")
		}
		if len(clusterNames) > 1 {
			log.Fatal("can run k9s on only 1 cluster at a time")
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		kc := kube.KubeconfigPath(clusterNames[0])
		command.Syscall("kubectl", append([]string{"--kubeconfig", kc}, args...))
	},
}
var rootKubectlCmd = &cobra.Command{
	Use:              kubeCtlCmd.Use,
	Aliases:          kubeCtlCmd.Aliases,
	Short:            kubeCtlCmd.Short,
	PersistentPreRun: kubeCmd.PersistentPreRun,
	PreRun:           kubeCtlCmd.PreRun,
	Run:              kubeCtlCmd.Run,
}

var kubeK9sCmd = &cobra.Command{
	Use:   "k9s",
	Short: "Runs k9s with the specified cluster",
	PreRun: func(cmd *cobra.Command, args []string) {
		if len(clusterNames) == 0 {
			log.Info("no cluster specifed, setting to controller")
			clusterNames = append(clusterNames, "controller")
		}
		if len(clusterNames) > 1 {
			log.Fatal("can run k9s on only 1 cluster at a time")
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		kc := kube.KubeconfigPath(clusterNames[0])
		command.Syscall("k9s", []string{"--kubeconfig", kc})
	},
}
var rootK9sCmd = &cobra.Command{
	Use:              kubeK9sCmd.Use,
	Short:            kubeK9sCmd.Short,
	PersistentPreRun: kubeCmd.PersistentPreRun,
	PreRun:           kubeK9sCmd.PreRun,
	Run:              kubeK9sCmd.Run,
}

func init() {
	kubeCmd.PersistentFlags().BoolVar(&kubeClusterControllerOnly, "controller",
		false, "Get controller cluster kubeconfig only")
	kubeCmd.PersistentFlags().IntVar(&kubeClusterTargetOnly, "target",
		0, "Get single target cluster kubeconfig")

	kubeCmd.AddCommand(kubeConfigCmd)
	kubeCmd.AddCommand(kubeCtlCmd)
	kubeCmd.AddCommand(kubeK9sCmd)
	rootCmd.AddCommand(kubeCmd)
	rootCmd.AddCommand(rootKubectlCmd)
	rootCmd.AddCommand(rootK9sCmd)
}
