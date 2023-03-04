// Package command provides an interface and implementations for shell commands
// which allow for easy testing and mocking.
//
// It follows the instructions at https://stackoverflow.com/a/74671137/351590
// and https://github.com/schollii/go-test-mock-exec-command which makes use
// of polymorphism to achieve proper testing and mocking.
package command

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"text/template"

	log "github.com/sirupsen/logrus"
)

// IShellCommand is an interface for running shell commands.
type IShellCommand interface {
	Run() error
	Output() ([]byte, error)
	CombinedOutput() ([]byte, error)
	RunProgressive() error
	SetDir(dir string)
	AddArgs(args ...string)
	Start() error
	SetStdin(in io.Reader)
	SetStdout(out io.Writer)
	StdoutPipe() (io.ReadCloser, error)
	StderrPipe() (io.ReadCloser, error)
	Wait() error
}

// ExecShellCommand implements IShellCommand.
type ExecShellCommand struct {
	*exec.Cmd
}

func (cmd ExecShellCommand) SetDir(dir string) {
	cmd.Dir = dir
}

func (cmd ExecShellCommand) AddArgs(args ...string) {
	cmd.Args = append(cmd.Args, args...)
}

func (cmd ExecShellCommand) RunProgressive() error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func (cmd ExecShellCommand) SetStdin(in io.Reader) {
	cmd.Stdin = in
}

func (cmd ExecShellCommand) SetStdout(out io.Writer) {
	cmd.Stdout = out
}

// NewExecShellCommander returns a command instance.
func NewExecShellCommander(name string, arg ...string) IShellCommand {
	execCmd := exec.Command(name, arg...)
	return ExecShellCommand{Cmd: execCmd}
}

// ShellCommander provides a wrapper around the commander to allow for better
// testing and mocking.
var ShellCommander = NewExecShellCommander

// GetMsgFromCommandError attempts to extract the error message from a command
// run's stderr.
func GetMsgFromCommandError(err error) error {
	var pathErr *fs.PathError
	var exitErr *exec.ExitError
	var errMsg string
	if errors.As(err, &pathErr) {
		errMsg = pathErr.Path + ": " + pathErr.Err.Error()
	} else if errors.As(err, &exitErr) {
		errMsg = string(exitErr.Stderr)
	} else {
		errMsg = err.Error()
	}
	return errors.New(errMsg)
}

func ScriptTemplate(tmpl string, vars interface{}) (string, error) {
	t, err := template.New("script").Parse(tmpl)
	if err != nil {
		return "", err
	}
	rendered := &bytes.Buffer{}
	if err = t.Execute(rendered, vars); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

// YesNoPrompt asks yes/no questions using the label.
func YesNoPrompt(label string, def bool) bool {
	choices := "Y/n"
	if !def {
		choices = "y/N"
	}

	r := bufio.NewReader(os.Stdin)
	var s string

	for {
		fmt.Fprintf(os.Stderr, "%s (%s) ", label, choices)
		s, _ = r.ReadString('\n')
		s = strings.TrimSpace(s)
		if s == "" {
			return def
		}
		s = strings.ToLower(s)
		if s == "y" || s == "yes" {
			return true
		}
		if s == "n" || s == "no" {
			return false
		}
	}
}

func Syscall(bin string, args []string) {
	binary, err := exec.LookPath(bin)
	if err != nil {
		panic(err)
	}

	execArgs := append([]string{bin}, args...)
	log.Info("running command: ", execArgs)
	log.Debugf("execArgs: %#v", execArgs)
	syscall.Exec(binary, execArgs, os.Environ())
}
