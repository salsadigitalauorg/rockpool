package rockpool

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/yusufhm/rockpool/internal"
)

func KubeApply(s *State, c *Config, fn string, force bool) {
	f, err := internal.RenderTemplate(fn, c.RenderedTemplatesPath, c)
	if err != nil {
		fmt.Printf("unable to render manifests for %s: %s", fn, err)
		os.Exit(1)
	}
	fmt.Println("using generated manifest at ", f)

	cmd := exec.Command(
		"kubectl", "--kubeconfig", s.Kubeconfig,
		"apply", "-f", f,
	)
	if force {
		cmd.Args = append(cmd.Args, "--force=true")
	}
	internal.RunCmdWithProgress(cmd)
}