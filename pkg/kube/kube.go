package kube

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/salsadigitalauorg/rockpool/internal"
	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	"github.com/salsadigitalauorg/rockpool/pkg/platform/templates"

	log "github.com/sirupsen/logrus"
)

func Cmd(cn string, ns string, args ...string) command.IShellCommand {
	cmd := command.ShellCommander("kubectl", "--kubeconfig", internal.KubeconfigPath(cn))
	if ns != "" {
		cmd.AddArgs("--namespace", ns)
	}
	cmd.AddArgs(args...)
	return cmd
}

func Apply(cn string, ns string, fn string, force bool) error {
	dryRun := Cmd(cn, ns, "apply", "-f", fn, "--dry-run=server")
	out, err := dryRun.Output()
	if err != nil {
		return err
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
	}).Info("applying manifest")
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
	logger.Info("applying generated manifest")
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
	cmd := Cmd(cn, ns, "exec", "deploy/"+deploy, "--", "bash", "-c", cmdStr)
	return cmd
}

func GetSecret(cn string, ns string, secret string, field string) ([]byte, string) {
	logger := log.WithFields(log.Fields{
		"clusterName": cn,
		"namespace":   ns,
		"secret":      secret,
		"field":       field,
	})
	logger.Info("fetching secret")

	cmd := Cmd(cn, ns, "get", "secret", secret, "--output")
	if field != "" {
		cmd.AddArgs(fmt.Sprintf("jsonpath='{.data.%s}'", field))
	} else {
		cmd.AddArgs("json")
	}

	out, err := cmd.Output()
	logger.WithField("out", string(out)).Debug()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("error getting secret")
	}

	if field != "" {
		logger.Info("decoding secret")
		out = []byte(strings.Trim(string(out), "'"))
		if decoded, err := base64.URLEncoding.DecodeString(string(out)); err != nil {
			logger.WithField("err", command.GetMsgFromCommandError(err)).
				Fatal("error decoding secret")
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
	logger.Info("fetching ConfigMap")

	cmd := Cmd(cn, ns, "get", "configmap", name, "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("error getting configmap")
	}
	return out
}

func Replace(cn string, ns string, name string, content string) string {
	logger := log.WithFields(log.Fields{
		"clusterName": cn,
		"namespace":   ns,
		"name":        name,
		"content":     content,
	})
	logger.Info("replacing manifest")

	cat := command.ShellCommander("echo", content)
	replace := Cmd(cn, ns, "replace", "-f", "-")

	reader, writer := io.Pipe()
	cat.SetStdout(writer)
	replace.SetStdin(reader)

	var replaceOut bytes.Buffer
	replace.SetStdout(&replaceOut)

	cat.Start()
	replace.Start()
	cat.Wait()
	writer.Close()

	if err := replace.Wait(); err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("error replacing manifest")
	}
	return replaceOut.String()
}

func Patch(cn string, ns string, kind string, name string, fn string) ([]byte, error) {
	logger := log.WithFields(log.Fields{
		"clusterName": cn,
		"namespace":   ns,
		"kind":        kind,
		"name":        name,
		"file":        fn,
	})
	logger.Info("applying patch")

	dryRun := Cmd(cn, ns, "patch", kind, name, "--patch-file", fn)
	dryRun.AddArgs("--dry-run=server")
	out, err := dryRun.Output()
	if err != nil {
		logger.WithField("err", command.GetMsgFromCommandError(err)).
			Fatal("error executing dry-run patch")
	}
	if strings.Contains(string(out), "(no change)") {
		return nil, nil
	}
	return Cmd(cn, ns, "patch", kind, name, "--patch-file", fn).Output()
}
