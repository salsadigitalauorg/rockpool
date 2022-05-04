package rockpool

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
)

func (r *Rockpool) VerifyReqs(failOnMissing bool) {
	binaries := []string{"k3d", "docker", "kubectl", "helm", "lagoon"}
	missing := []string{}
	r.State.BinaryPaths = map[string]string{}
	for _, b := range binaries {
		path, err := exec.LookPath(b)
		if err != nil {
			missing = append(missing, fmt.Sprintf("could not find %s; please ensure it is installed before", b))
			continue
		}
		r.State.BinaryPaths[b] = path
	}
	if failOnMissing {
		for _, m := range missing {
			fmt.Println(m)
		}
	}
	if failOnMissing && len(missing) > 0 {
		fmt.Println("some requirements were not met; please review above")
		os.Exit(1)
	}

	// Create temporary directory for rendered templates.
	err := os.MkdirAll(r.Config.RenderedTemplatesPath, os.ModePerm)
	if err != nil {
		fmt.Printf("unable to create temp dir %s: %s\n", r.Config.RenderedTemplatesPath, err)
		os.Exit(1)
	}
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

func (r *Rockpool) UpdateState() {
	r.VerifyReqs(false)
	r.FetchClusters()
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
			if n.Role != "loadbalancer" {
				return n.IP.IP
			}
		}
	}
	fmt.Println("unable to get controller ip")
	os.Exit(1)
	return ""
}

func (r *Rockpool) ControllerClusterName() string {
	return r.Config.ClusterName + "-controller"
}

func (r *Rockpool) TargetClusterName(targetId int) string {
	return r.Config.ClusterName + "-target-" + fmt.Sprint(targetId)
}