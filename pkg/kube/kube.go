package kube

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	"github.com/salsadigitalauorg/rockpool/pkg/platform/templates"

	log "github.com/sirupsen/logrus"
)

func KubeconfigPath(clusterName string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Sprintln("unable to get user home directory:", err))
	}
	return fmt.Sprintf("%s/.k3d/kubeconfig-%s.yaml", home, clusterName)
}

func GetTargetIdFromCn(cn string) int {
	cnParts := strings.Split(cn, "-")
	idStr := cnParts[len(cnParts)-1]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.WithFields(log.Fields{
			"cluster": cn,
			"id":      idStr,
		}).Fatal("invalid cluster id")
	}
	return id
}

func Cmd(cn string, ns string, args ...string) command.IShellCommand {
	cmd := command.ShellCommander("kubectl", "--kubeconfig", KubeconfigPath(cn))
	if ns != "" {
		cmd.AddArgs("--namespace", ns)
	}
	cmd.AddArgs(args...)
	return cmd
}

func Apply(cn string, ns string, fn string, force bool) error {
	// dry-run first to check for changes.
	out, err := Cmd(cn, ns, "apply", "-f", fn, "--dry-run=server").Output()
	if err != nil {
		return command.GetMsgFromCommandError(err)
	}
	changesRequired := false
	for _, l := range strings.Split(strings.Trim(string(out), "\n"), "\n") {
		if !strings.Contains(l, "unchanged (server dry run)") {
			changesRequired = true
			break
		}
	}
	if !changesRequired {
		return nil
	}

	cmd := Cmd(cn, ns, "apply", "-f", fn)
	if force {
		cmd.AddArgs("--force=true")
	}
	log.WithFields(log.Fields{
		"clusterName": cn,
		"namespace":   ns,
		"file":        fn,
		"force":       force,
	}).Debug("applying manifest")
	return cmd.RunProgressive()
}

func ApplyTemplate(cn string, ns string, fn string, force bool, retries int, delay int) {
	logger := log.WithFields(log.Fields{
		"clusterName": cn,
		"namespace":   ns,
		"file":        fn,
		"force":       force,
	})

	f, err := templates.Render(fn, platform.ToMap(), "")
	if err != nil {
		logger.Fatal("unable to render template")
	}
	logger.Debug("applying generated manifest")
	err = Apply(cn, ns, f, force)
	if err != nil {
		var failedErr error
		if retries > 0 {
			failed := true
			for retries > 0 && failed {
				failedErr = nil
				err = Apply(cn, ns, f, force)
				if err != nil {
					failed = true
					failedErr = err
					retries--
					time.Sleep(time.Duration(delay) * time.Second)
					continue
				}
				failed = false
			}
		}

		if failedErr != nil {
			logger.WithField("retryFailedErr", failedErr)
		}

		logger.Fatal("unable to apply generated template")
	}

}

func Exec(cn string, ns string, deploy string, cmdStr string) command.IShellCommand {
	return Cmd(cn, ns, "exec", "deploy/"+deploy, "--", "bash", "-c", cmdStr)
}

func GetSecret(cn string, ns string, secret string, field string) ([]byte, string) {
	logger := log.WithFields(log.Fields{
		"clusterName": cn,
		"namespace":   ns,
		"secret":      secret,
		"field":       field,
	})
	logger.Debug("fetching secret")

	cmd := Cmd(cn, ns, "get", "secret", secret, "--output")
	if field != "" {
		cmd.AddArgs(fmt.Sprintf("jsonpath='{.data.%s}'", field))
	} else {
		cmd.AddArgs("json")
	}

	out, err := cmd.Output()
	logger.WithField("out", string(out)).Debug()
	if err != nil {
		logger.WithError(command.GetMsgFromCommandError(err)).
			Fatal("error getting secret")
	}

	if field != "" {
		logger.Debug("decoding secret")
		out = []byte(strings.Trim(string(out), "'"))
		if decoded, err := base64.URLEncoding.DecodeString(string(out)); err != nil {
			logger.WithError(err).Fatal("error decoding secret")
		} else {
			return nil, string(decoded)
		}
	}
	return out, ""
}

func GetConfigMap(cn string, ns string, name string) []byte {
	logger := log.WithFields(log.Fields{
		"clusterName": cn,
		"namespace":   ns,
		"name":        name,
	})
	logger.Debug("fetching ConfigMap")

	cmd := Cmd(cn, ns, "get", "configmap", name, "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		logger.WithError(command.GetMsgFromCommandError(err)).
			Fatal("error getting configmap")
	}
	return out
}

func Replace(cn string, ns string, name string, content string) {
	logger := log.WithFields(log.Fields{
		"clusterName": cn,
		"namespace":   ns,
		"name":        name,
		"content":     content,
	})
	logger.Debug("replacing manifest")

	cat := command.ShellCommander("echo", content)
	replace := Cmd(cn, ns, "replace", "-f", "-")

	reader, writer := io.Pipe()
	cat.SetStdout(writer)
	replace.SetStdin(reader)

	replace.SetStdout(os.Stdout)

	cat.Start()
	replace.Start()
	cat.Wait()
	writer.Close()

	if err := replace.Wait(); err != nil {
		logger.WithError(command.GetMsgFromCommandError(err)).
			Fatal("error replacing manifest")
	}
}

func Patch(cn string, ns string, kind string, name string, fn string) ([]byte, error) {
	logger := log.WithFields(log.Fields{
		"clusterName": cn,
		"namespace":   ns,
		"kind":        kind,
		"name":        name,
		"file":        fn,
	})
	logger.Debug("applying patch")

	out, err := Cmd(cn, ns, "patch", kind, name, "--patch-file", fn,
		"--dry-run=server").Output()
	if err != nil {
		logger.WithError(command.GetMsgFromCommandError(err)).
			Fatal("error executing dry-run patch")
	}
	if strings.Contains(string(out), "(no change)") {
		return nil, nil
	}
	return Cmd(cn, ns, "patch", kind, name, "--patch-file", fn).Output()
}
