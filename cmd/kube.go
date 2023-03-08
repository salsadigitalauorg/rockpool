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

var kubeConfigClusterControllerOnly bool
var kubeConfigClusterTargetOnly int
var kubeClusterControllerOnly bool
var kubeClusterTargetOnly int
var clusterNames []string
var clusterName string

var kubeCmd = &cobra.Command{
	Use:   "kube [command]",
	Short: "Execute kubernetes operations.",
}

var kubeConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Outputs the kubeconfig path for the cluster(s)",
	PreRun: func(cmd *cobra.Command, args []string) {
		if kubeConfigClusterControllerOnly {
			clusterNames = append(clusterNames, platform.ControllerClusterName())
			return
		}
		if kubeConfigClusterTargetOnly > 0 {
			clusterNames = append(clusterNames, platform.TargetClusterName(kubeClusterTargetOnly))
			return
		}

		k3d.ClusterFetch()
		if len(k3d.Clusters) > 1 {
			for _, c := range k3d.Clusters {
				clusterNames = append(clusterNames, c.Name)
			}
		}

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
		if kubeClusterControllerOnly {
			clusterName = platform.ControllerClusterName()
		}
		if kubeClusterTargetOnly > 0 {
			clusterName = platform.TargetClusterName(kubeClusterTargetOnly)
		}
		if clusterName == "" {
			log.Fatal("no cluster specified - use one of --controller or --target=1")
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		k3d.ClusterFetch()
		kc := kube.KubeconfigPath(clusterName)
		command.Syscall("kubectl", append([]string{"--kubeconfig", kc}, args...))
	},
}
var rootKubectlCmd = &cobra.Command{
	Use:     kubeCtlCmd.Use,
	Aliases: kubeCtlCmd.Aliases,
	Short:   kubeCtlCmd.Short,
	PreRun:  kubeCtlCmd.PreRun,
	Run:     kubeCtlCmd.Run,
}

var kubeK9sCmd = &cobra.Command{
	Use:    "k9s",
	Short:  "Runs k9s with the specified cluster",
	PreRun: kubeCtlCmd.PreRun,
	Run: func(cmd *cobra.Command, args []string) {
		k3d.ClusterFetch()
		kc := kube.KubeconfigPath(clusterName)
		command.Syscall("k9s", []string{"--kubeconfig", kc})
	},
}
var rootK9sCmd = &cobra.Command{
	Use:    kubeK9sCmd.Use,
	Short:  kubeK9sCmd.Short,
	PreRun: kubeK9sCmd.PreRun,
	Run:    kubeK9sCmd.Run,
}

func init() {
	kubeConfigCmd.Flags().BoolVar(&kubeConfigClusterControllerOnly, "controller",
		false, "Get controller cluster kubeconfig only")
	kubeConfigCmd.Flags().IntVar(&kubeConfigClusterTargetOnly, "target",
		0, "Get single target cluster kubeconfig")

	kubeCtlCmd.Flags().BoolVar(&kubeClusterControllerOnly, "controller",
		true, "Get controller cluster kubeconfig only")
	kubeCtlCmd.Flags().IntVar(&kubeClusterTargetOnly, "target",
		0, "Get single target cluster kubeconfig")
	rootKubectlCmd.Flags().BoolVar(&kubeClusterControllerOnly, "controller",
		true, "Get controller cluster kubeconfig only")
	rootKubectlCmd.Flags().IntVar(&kubeClusterTargetOnly, "target",
		0, "Get single target cluster kubeconfig")

	kubeK9sCmd.Flags().BoolVar(&kubeClusterControllerOnly, "controller",
		true, "Get controller cluster kubeconfig only")
	kubeK9sCmd.Flags().IntVar(&kubeClusterTargetOnly, "target",
		0, "Get single target cluster kubeconfig")
	rootK9sCmd.Flags().BoolVar(&kubeClusterControllerOnly, "controller",
		true, "Get controller cluster kubeconfig only")
	rootK9sCmd.Flags().IntVar(&kubeClusterTargetOnly, "target",
		0, "Get single target cluster kubeconfig")

	kubeCmd.AddCommand(kubeConfigCmd)
	kubeCmd.AddCommand(kubeCtlCmd)
	kubeCmd.AddCommand(kubeK9sCmd)

	rootCmd.AddCommand(kubeCmd)
	rootCmd.AddCommand(rootKubectlCmd)
	rootCmd.AddCommand(rootK9sCmd)
}
