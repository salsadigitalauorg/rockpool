package rockpool

import (
	"fmt"
	"os"
	"os/exec"
)

func VerifyReqs(s *State) {
	binaries := []string{"k3d", "docker", "kubectl", "helm", "lagoon"}
	missing := []string{}
	s.BinaryPaths = map[string]string{}
	for _, b := range binaries {
		path, err := exec.LookPath(b)
		if err != nil {
			missing = append(missing, fmt.Sprintf("could not find %s; please ensure it is installed before", b))
			continue
		}
		fmt.Printf("%s is available at %s\n", b, path)
		s.BinaryPaths[b] = path
	}
	for _, m := range missing {
		fmt.Printf(m)
	}
	if len(missing) > 0 {
		fmt.Println("some requirements were not met; please review above")
		os.Exit(1)
	}
}