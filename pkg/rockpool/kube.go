package rockpool

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/yusufhm/rockpool/internal"
)

func (r *Rockpool) KubeCtl(cn string, ns string, args ...string) *exec.Cmd {
	cmd := exec.Command("kubectl", "--kubeconfig", r.State.Kubeconfig[cn])
	if ns != "" {
		cmd.Args = append(cmd.Args, "--namespace", ns)
	}
	cmd.Args = append(cmd.Args, args...)
	return cmd
}

func (r *Rockpool) KubeApply(cn string, ns string, fn string, force bool) {
	f, err := internal.RenderTemplate(fn, r.Config.RenderedTemplatesPath, r.Config)
	if err != nil {
		fmt.Printf("unable to render manifests for %s: %s", fn, err)
		os.Exit(1)
	}
	fmt.Println("using generated manifest at ", f)

	cmd := r.KubeCtl(cn, ns, "apply", "-f", f)
	if force {
		cmd.Args = append(cmd.Args, "--force=true")
	}
	internal.RunCmdWithProgress(cmd)
}

func (r *Rockpool) KubeExec(cn string, ns string, deploy string, cmdStr string) error {
	cmd := r.KubeCtl(cn, ns, "exec", "deploy/"+deploy, "--", "bash", "-c", cmdStr)
	fmt.Println("kube exec command: ", cmd)
	return internal.RunCmdWithProgress(cmd)
}

func (r *Rockpool) KubeGetSecret(cn string, ns string, secret string, field string) string {
	cmd := r.KubeCtl(
		cn, ns, "get", "secret", secret,
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
