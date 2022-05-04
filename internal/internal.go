package internal

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

// RunCmdWithProgress runs a command and progressively outputs the progress.
func RunCmdWithProgress(cmd *exec.Cmd) ([]byte, error) {
	// Use pipes so we can output progress.
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	_ = cmd.Start()

	var stdoutBytes []byte
	scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		m := scanner.Text()
		fmt.Println(m)
		stdoutBytes = append(stdoutBytes, scanner.Bytes()...)
	}
	return stdoutBytes, cmd.Wait()
}

// RenderTemplate executes a given template file and returns the path to its
// rendered version.
func RenderTemplate(tn string, path string, config interface{}, destName string) (string, error) {
	tnFull := fmt.Sprintf("templates/%s", tn)
	t := template.Must(template.ParseFiles(tnFull))

	var rendered string
	if destName != "" {
		rendered = filepath.Join(path, destName)
	} else if filepath.Ext(tn) == ".tmpl" {
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

func GetTargetIdFromCn(cn string) int {
	cnParts := strings.Split(cn, "-")
	idStr := cnParts[len(cnParts)-1]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		fmt.Println("invalid cluster id: ", idStr)
		os.Exit(1)
	}
	return id
}
