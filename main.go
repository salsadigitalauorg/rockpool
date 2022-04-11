package main

import (
	"fmt"

	r "github.com/yusufhm/rockpool/pkg/rockpool"
)

var state r.State
var config r.Config

func main() {
	state = r.State{}
	r.VerifyReqs(&state)
	fmt.Println()

	config = r.Config{
		ClusterName:   "rockpool",
		LagoonBaseUrl: "lagoon.rockpool.k3d.local",
		HarborPass:    "pass",
	}
	r.CreateCluster(&state, config.ClusterName)
	fmt.Println()
	r.GetClusterKubeConfigPath(&state, config.ClusterName)

	r.ClusterVersion(&state)
	fmt.Println()
	r.HelmList(&state)
	r.InstallIngressNginx(&state)

	r.InstallHarbor(&state, &config)
	r.InstallLagoonCore(&state, &config)
}
