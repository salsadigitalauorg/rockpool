package rockpool

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/yusufhm/rockpool/internal"
)

func KubeCtl(s *State, args ...string) *exec.Cmd {
	cmd := exec.Command("kubectl", "--kubeconfig", s.Kubeconfig)
	cmd.Args = append(cmd.Args, args...)
	return cmd
}

func KubeApply(s *State, c *Config, fn string, force bool) {
	f, err := internal.RenderTemplate(fn, c.RenderedTemplatesPath, c)
	if err != nil {
		fmt.Printf("unable to render manifests for %s: %s", fn, err)
		os.Exit(1)
	}
	fmt.Println("using generated manifest at ", f)

	cmd := KubeCtl(s, "apply", "-f", f)
	if force {
		cmd.Args = append(cmd.Args, "--force=true")
	}
	internal.RunCmdWithProgress(cmd)
}

func KubeExec(s *State, namespace string, deploy string, cmdStr string) error {
	cmd := KubeCtl(
		s, "exec", "--namespace", namespace,
		"deploy/"+deploy, "--", "bash", "-c", cmdStr,
	)
	fmt.Println("kube exec command: ", cmd)
	return internal.RunCmdWithProgress(cmd)
}

func KubeGetSecret(s *State, namespace string, secret string, field string) string {
	cmd := KubeCtl(
		s, "--namespace", namespace, "get", "secret", secret,
		"--output", fmt.Sprintf("jsonpath='{.data.%s}'", field),
	)
	out, err := cmd.Output()
	if err != nil {
		fmt.Printf("error when getting secret %s: %s", secret, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	out = []byte(strings.Trim(string(out), "'"))
	if decoded, err := base64.URLEncoding.DecodeString(string(out)); err != nil {
		fmt.Printf("error when decoding secret %s: %#v", secret, internal.GetCmdStdErr(err))
		os.Exit(1)
	} else {
		return string(decoded)
	}
	return ""
}
