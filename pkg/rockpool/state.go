package rockpool

import (
	"fmt"

	"github.com/salsadigitalauorg/rockpool/internal"
	"github.com/salsadigitalauorg/rockpool/pkg/k3d"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
)

func (r *Rockpool) Status() {
	k3d.ClusterFetch()
	if len(k3d.Clusters) == 0 {
		fmt.Printf("No cluster found for '%s'\n", platform.Name)
		return
	}

	runningClusters := 0
	fmt.Println("Clusters:")
	for _, c := range k3d.Clusters {
		isRunning := k3d.ClusterIsRunning(c.Name)
		fmt.Printf("  %s: ", c.Name)
		if isRunning {
			fmt.Println("running")
			runningClusters++
		} else {
			fmt.Println("stopped")
		}
	}

	if runningClusters == 0 {
		fmt.Println("No running cluster")
		return
	}

	fmt.Println("Kubeconfig:")
	fmt.Println("  Controller:", internal.KubeconfigPath(platform.ControllerClusterName()))
	if len(k3d.Clusters) > 1 {
		fmt.Println("  Targets:")
		for _, c := range k3d.Clusters {
			if c.Name == platform.ControllerClusterName() {
				continue
			}
			fmt.Println("    ", internal.KubeconfigPath(c.Name))
		}
	}

	fmt.Println("Gitea:")
	fmt.Printf("  http://gitea.lagoon.%s\n", platform.Hostname())
	fmt.Println("  User: rockpool")
	fmt.Println("  Pass: pass")

	fmt.Println("Keycloak:")
	fmt.Printf("  http://keycloak.lagoon.%s/auth/admin\n", platform.Hostname())
	fmt.Println("  User: admin")
	fmt.Println("  Pass: pass")

	fmt.Printf("Lagoon UI: http://ui.lagoon.%s\n", platform.Hostname())
	fmt.Println("  User: lagoonadmin")
	fmt.Println("  Pass: pass")

	fmt.Printf("Lagoon GraphQL: http://api.lagoon.%s/graphql\n", platform.Hostname())
	fmt.Println("Lagoon SSH: ssh -p 2022 lagoon@localhost")

	fmt.Println()
}
