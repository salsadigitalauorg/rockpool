package kube

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/salsadigitalauorg/rockpool/internal"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	"github.com/salsadigitalauorg/rockpool/pkg/templates"
)

func Cmd(cn string, ns string, args ...string) *exec.Cmd {
	cmd := exec.Command("kubectl", "--kubeconfig", internal.KubeconfigPath(cn))
	if ns != "" {
		cmd.Args = append(cmd.Args, "--namespace", ns)
	}
	cmd.Args = append(cmd.Args, args...)
	return cmd
}

func Apply(cn string, ns string, fn string, force bool) ([]byte, error) {
	dryRun := Cmd(cn, ns, "apply", "-f", fn, "--dry-run=server")
	out, err := dryRun.Output()
	if err != nil {
		return nil, err
	}
	changesRequired := false
	for _, l := range strings.Split(strings.Trim(string(out), "\n"), "\n") {
		if !strings.Contains(l, "unchanged (server dry run)") {
			changesRequired = true
			break
		}
	}
	if !changesRequired {
		return nil, nil
	}

	cmd := Cmd(cn, ns, "apply", "-f", fn)
	if force {
		cmd.Args = append(cmd.Args, "--force=true")
	}
	return cmd.Output()
}

func ApplyTemplate(cn string, ns string, fn string, force bool) ([]byte, error) {
	f, err := templates.Render(fn, platform.ToMap(), "")
	if err != nil {
		return nil, err
	}
	fmt.Printf("[%s] using generated manifest at %s\n", cn, f)
	return Apply(cn, ns, f, force)
}

func ExecNoProgress(cn string, ns string, deploy string, cmdStr string) *exec.Cmd {
	cmd := Cmd(cn, ns, "exec", "deploy/"+deploy, "--", "bash", "-c", cmdStr)
	return cmd
}

func Exec(cn string, ns string, deploy string, cmdStr string) ([]byte, error) {
	cmd := ExecNoProgress(cn, ns, deploy, cmdStr)
	fmt.Printf("[%s] kube exec command: %s\n", cn, cmd)
	return cmd.Output()
}

func GetSecret(cn string, ns string, secret string, field string) ([]byte, string) {
	cmd := Cmd(cn, ns, "get", "secret", secret, "--output")
	if field != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("jsonpath='{.data.%s}'", field))
	} else {
		cmd.Args = append(cmd.Args, "json")
	}
	out, err := cmd.Output()
	if err != nil {
		fmt.Printf("[%s] error when getting secret %s: %s\n", cn, secret, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	if field != "" {
		out = []byte(strings.Trim(string(out), "'"))
		if decoded, err := base64.URLEncoding.DecodeString(string(out)); err != nil {
			fmt.Printf("[%s] error when decoding secret %s: %#v\n", cn, secret, internal.GetCmdStdErr(err))
			os.Exit(1)
		} else {
			return nil, string(decoded)
		}
	}
	return out, ""
}

func GetConfigMap(cn string, ns string, name string) []byte {
	cmd := Cmd(
		cn, ns, "get", "configmap", name,
		"--output", "json",
	)
	out, err := cmd.Output()
	if err != nil {
		fmt.Printf("[%s] error when getting configmap %s: %s\n", cn, name, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	return out
}

func Replace(cn string, ns string, name string, content string) string {
	cat := exec.Command("echo", content)
	replace := Cmd(cn, ns, "replace", "-f", "-")

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
		fmt.Printf("[%s] error replacing config %s: %s\n", cn, name, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	return replaceOut.String()
}

func Patch(cn string, ns string, kind string, name string, fn string) ([]byte, error) {
	dryRun := Cmd(cn, ns, "patch", kind, name, "--patch-file", fn)
	dryRun.Args = append(dryRun.Args, "--dry-run=server")
	out, err := dryRun.Output()
	if err != nil {
		fmt.Printf("[%s] error executing dry-run patch: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	if strings.Contains(string(out), "(no change)") {
		return nil, nil
	}
	return Cmd(cn, ns, "patch", kind, name, "--patch-file", fn).Output()
}
