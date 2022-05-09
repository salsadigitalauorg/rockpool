package rockpool

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
)

func (r *Rockpool) VerifyReqs(failOnMissing bool) {
	binaries := []string{"k3d", "docker", "kubectl", "helm", "lagoon"}
	missing := []string{}
	r.State.BinaryPaths = sync.Map{}
	for _, b := range binaries {
		path, err := exec.LookPath(b)
		if err != nil {
			missing = append(missing, fmt.Sprintf("[rockpool] could not find %s; please ensure it is installed and can be found in the $PATH", b))
			continue
		}
		r.State.BinaryPaths.Store(b, path)
	}
	if failOnMissing {
		for _, m := range missing {
			fmt.Println(m)
		}
	}
	if failOnMissing && len(missing) > 0 {
		fmt.Println("[rockpool] some requirements were not met; please review above")
		os.Exit(1)
	}

	// Create directory for rendered templates.
	err := os.MkdirAll(r.RenderedTemplatesPath(), os.ModePerm)
	if err != nil {
		fmt.Printf("[rockpool] unable to create temp dir %s: %s\n", r.RenderedTemplatesPath(), err)
		os.Exit(1)
	}
}

func (r *Rockpool) Kubeconfig(cn string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Sprintln("unable to get user home directory:", err))
	}
	return fmt.Sprintf("%s/.k3d/kubeconfig-%s.yaml", home, cn)
}

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

func (r *Rockpool) GetHelmReleases(key string) []HelmRelease {
	valueIfc, ok := r.HelmReleases.Load(key)
	if !ok {
		panic(fmt.Sprint("releases not found for ", key))
	}
	val, ok := valueIfc.([]HelmRelease)
	if !ok {
		panic(fmt.Sprint("unable to convert binpath to string for ", valueIfc))
	}
	return val
}

func (r *Rockpool) GetBinaryPath(bin string) string {
	return r.MapStringGet(&r.State.BinaryPaths, bin)
}

func (r *Rockpool) WgAdd(delta int) {
	if r.wg == nil {
		r.wg = &sync.WaitGroup{}
	}
	r.wg.Add(delta)
}

func (r *Rockpool) WgWait() {
	if r.wg != nil {
		r.wg.Wait()
		r.wg = nil
	}
}

func (r *Rockpool) WgDone() {
	if r.wg != nil {
		r.wg.Done()
	}
}

func (r *Rockpool) FetchClusters() {
	var allK3dCl ClusterList
	allK3dCl.Get()
	for _, c := range allK3dCl {
		if !strings.HasPrefix(c.Name, r.ClusterName) {
			continue
		}
		if exists, _ := r.Clusters.ClusterExists(c.Name); exists {
			continue
		}
		r.Clusters = append(r.Clusters, c)
	}
}

func (r *Rockpool) Status() {
	r.FetchClusters()
	fmt.Println("Kubeconfig:")
	fmt.Println("  Controller:", r.Kubeconfig(r.ControllerClusterName()))
	if len(r.State.Clusters) > 2 {
		fmt.Println("  Targets:")
		for _, c := range r.State.Clusters {
			if c.Name == r.ControllerClusterName() {
				continue
			}
			fmt.Println("    ", r.Kubeconfig(c.Name))
		}
	}

	fmt.Println("Gitea:")
	fmt.Printf("  http://gitea.%s\n", r.Hostname)
	fmt.Println("  User: rockpool")
	fmt.Println("  Pass: pass")

	fmt.Println("Keycloak:")
	fmt.Printf("  %s/admin\n", r.KeycloakUrl())
	fmt.Println("  User: admin")
	fmt.Println("  Pass: pass")

	fmt.Printf("Lagoon UI: http://ui.lagoon.%s\n", r.Hostname)
	fmt.Println("  User: lagoonadmin")
	fmt.Println("  Pass: pass")

	fmt.Printf("Lagoon GraphQL: http://api.lagoon.%s/graphql\n", r.Hostname)
	fmt.Println("Lagoon SSH: ssh -p 2022 lagoon@localhost")

	fmt.Println()
}

func (r *Rockpool) TotalClusterNum() int {
	return r.Config.NumTargets + 1
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
	return r.Config.ClusterName + "-controller"
}

func (r *Rockpool) TargetClusterName(targetId int) string {
	return r.Config.ClusterName + "-target-" + fmt.Sprint(targetId)
}

func (r *Rockpool) RenderedTemplatesPath() string {
	return path.Join(r.Config.ConfigDir, "rendered", r.Config.ClusterName)
}

func (r *Rockpool) KeycloakUrl() string {
	return fmt.Sprintf("http://keycloak.%s/auth", r.Config.LagoonBaseUrl)
}
