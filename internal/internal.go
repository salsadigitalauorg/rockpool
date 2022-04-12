package internal

import (
	"bufio"
	"fmt"
	"html/template"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunCmdWithProgress runs a command and progressively outputs the progress.
func RunCmdWithProgress(cmd *exec.Cmd) error {
	// Use pipes so we can output progress.
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	_ = cmd.Start()

	scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		m := scanner.Text()
		fmt.Println(m)
	}
	return cmd.Wait()
}

// RenderTemplate executes a given template file and returns the path to its
// rendered version.
func RenderTemplate(tn string, path string, config interface{}) (string, error) {
	tnFull := fmt.Sprintf("templates/%s", tn)
	t := template.Must(template.ParseFiles(tnFull))

	var rendered string
	if filepath.Ext(tn) == ".tmpl" {
		rendered = filepath.Join(path, strings.TrimSuffix(tn, ".tmpl"))
	} else {
		rendered = filepath.Join(path, tn)
	}

	f, err := os.Create(rendered)
	if err != nil {
		return "", err
	}

	err = t.Execute(f, config)
	f.Close()
	if err != nil {
		return "", err
	}
	return rendered, nil
}

// GetCmdStdErr extracts the error from a failed command's err.
func GetCmdStdErr(err error) string {
	if exitError, ok := err.(*exec.ExitError); ok {
		return string(exitError.Stderr)
	}
	return err.Error()
}
