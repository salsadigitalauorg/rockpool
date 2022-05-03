package rockpool

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
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

func (r *Rockpool) KubeApply(cn string, ns string, fn string, force bool) (string, error) {
	dryRun := r.KubeCtl(cn, ns, "apply", "-f", fn, "--dry-run=server")
	out, err := dryRun.Output()
	if err != nil {
		fmt.Println("error executing dry-run apply:", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	changesRequired := false
	for _, l := range strings.Split(strings.Trim(string(out), "\n"), "\n") {
		if !strings.Contains(l, "unchanged (server dry run)") {
			changesRequired = true
			break
		}
	}
	if !changesRequired {
		return "", nil
	}

	cmd := r.KubeCtl(cn, ns, "apply", "-f", fn)
	if force {
		cmd.Args = append(cmd.Args, "--force=true")
	}
	return internal.RunCmdWithProgress(cmd)
}

func (r *Rockpool) KubeApplyTemplate(cn string, ns string, fn string, force bool) (string, error) {
	f, err := internal.RenderTemplate(fn, r.Config.RenderedTemplatesPath, r.Config)
	if err != nil {
		fmt.Printf("unable to render manifests for %s: %s", fn, err)
		os.Exit(1)
	}
	fmt.Println("using generated manifest at", f)
	return r.KubeApply(cn, ns, f, force)
}

func (r *Rockpool) KubeExecNoProgress(cn string, ns string, deploy string, cmdStr string) *exec.Cmd {
	cmd := r.KubeCtl(cn, ns, "exec", "deploy/"+deploy, "--", "bash", "-c", cmdStr)
	return cmd
}

func (r *Rockpool) KubeExec(cn string, ns string, deploy string, cmdStr string) (string, error) {
	cmd := r.KubeExecNoProgress(cn, ns, deploy, cmdStr)
	fmt.Println("kube exec command: ", cmd)
	return internal.RunCmdWithProgress(cmd)
}

func (r *Rockpool) KubeGetSecret(cn string, ns string, secret string, field string) ([]byte, string) {
	cmd := r.KubeCtl(cn, ns, "get", "secret", secret, "--output")
	if field != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("jsonpath='{.data.%s}'", field))
	} else {
		cmd.Args = append(cmd.Args, "json")
	}
	out, err := cmd.Output()
	if err != nil {
		fmt.Printf("error when getting secret %s: %s", secret, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	if field != "" {
		out = []byte(strings.Trim(string(out), "'"))
		if decoded, err := base64.URLEncoding.DecodeString(string(out)); err != nil {
			fmt.Printf("error when decoding secret %s: %#v", secret, internal.GetCmdStdErr(err))
			os.Exit(1)
		} else {
			return nil, string(decoded)
		}
	}
	return out, ""
}

func (r *Rockpool) KubeGetConfigMap(cn string, ns string, name string) []byte {
	cmd := r.KubeCtl(
		cn, ns, "get", "configmap", name,
		"--output", "json",
	)
	fmt.Println(cmd)
	out, err := cmd.Output()
	if err != nil {
		fmt.Printf("error when getting configmap %s: %s", name, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	return out
}

func (r *Rockpool) KubeReplace(cn string, ns string, name string, content string) string {
	cat := exec.Command("echo", content)
	replace := r.KubeCtl(cn, ns, "replace", "-f", "-")

	reader, writer := io.Pipe()
	cat.Stdout = writer
	replace.Stdin = reader

	var replaceOut bytes.Buffer
	replace.Stdout = &replaceOut

	cat.Start()
	replace.Start()
	cat.Wait()
	writer.Close()

	if err := replace.Wait(); err != nil {
		fmt.Printf("error replacing config %s: %s", name, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	io.Copy(os.Stdout, &replaceOut)
	return replaceOut.String()
}

func (r *Rockpool) KubePatch(cn string, ns string, kind string, name string, fn string) ([]byte, error) {
	dryRun := r.KubeCtl(cn, ns, "patch", kind, name, "--patch-file", fn)
	dryRun.Args = append(dryRun.Args, "--dry-run=server")
	out, err := dryRun.Output()
	if err != nil {
		fmt.Println("error executing dry-run patch:", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	if strings.Contains(string(out), "(no change)") {
		return nil, nil
	}
	return r.KubeCtl(cn, ns, "patch", kind, name, "--patch-file", fn).Output()
}
