package rockpool

import (
	"fmt"
	"os"
	"sync"

	"github.com/salsadigitalauorg/rockpool/internal"
)

func (r *Rockpool) MapStringGet(m *sync.Map, key string) string {
	valueIfc, ok := m.Load(key)
	if !ok {
		panic(fmt.Sprint("value not found for ", key))
	}
	val, ok := valueIfc.(string)
	if !ok {
		panic(fmt.Sprint("unable to convert interface{} value to string for ", valueIfc))
	}
	return val
}

func (r *Rockpool) Status() {
	r.K3d.ClusterFetch()
	if len(r.K3d.Clusters) == 0 {
		fmt.Printf("No cluster found for '%s'\n", r.Name)
		return
	}

	runningClusters := 0
	fmt.Println("Clusters:")
	for _, c := range r.K3d.Clusters {
		isRunning := r.K3d.ClusterIsRunning(c.Name)
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
	fmt.Println("  Controller:", internal.KubeconfigPath(r.ControllerClusterName()))
	if len(r.K3d.Clusters) > 1 {
		fmt.Println("  Targets:")
		for _, c := range r.K3d.Clusters {
			if c.Name == r.ControllerClusterName() {
				continue
			}
			fmt.Println("    ", internal.KubeconfigPath(c.Name))
		}
	}

	fmt.Println("Gitea:")
	fmt.Printf("  http://gitea.lagoon.%s\n", r.Hostname())
	fmt.Println("  User: rockpool")
	fmt.Println("  Pass: pass")

	fmt.Println("Keycloak:")
	fmt.Printf("  http://keycloak.lagoon.%s/auth/admin\n", r.Hostname())
	fmt.Println("  User: admin")
	fmt.Println("  Pass: pass")

	fmt.Printf("Lagoon UI: http://ui.lagoon.%s\n", r.Hostname())
	fmt.Println("  User: lagoonadmin")
	fmt.Println("  Pass: pass")

	fmt.Printf("Lagoon GraphQL: http://api.lagoon.%s/graphql\n", r.Hostname())
	fmt.Println("Lagoon SSH: ssh -p 2022 lagoon@localhost")

	fmt.Println()
}

func (r *Rockpool) TotalClusterNum() int {
	return r.Config.NumTargets + 1
}

func (r *Rockpool) Hostname() string {
	return fmt.Sprintf("%s.%s", r.Name, r.Domain)
}

func (r *Rockpool) ControllerIP() string {
	for _, c := range r.Clusters {
		if c.Name != r.ControllerClusterName() {
			continue
		}

		for _, n := range c.Nodes {
			if n.Role == "loadbalancer" {
				return n.IP.IP
			}
		}
	}
	fmt.Println("[rockpool] unable to get controller ip")
	os.Exit(1)
	return ""
}

func (r *Rockpool) TargetIP(cn string) string {
	for _, c := range r.Clusters {
		if c.Name != cn {
			continue
		}

		for _, n := range c.Nodes {
			if n.Role == "loadbalancer" {
				return n.IP.IP
			}
		}
	}
	fmt.Println("[rockpool] unable to get target ip")
	os.Exit(1)
	return ""
}

func (r *Rockpool) ControllerClusterName() string {
	return r.Config.Name + "-controller"
}

func (r *Rockpool) TargetClusterName(targetId int) string {
	return r.Config.Name + "-target-" + fmt.Sprint(targetId)
}
