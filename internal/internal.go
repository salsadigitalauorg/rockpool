package internal

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func KubeconfigPath(clusterName string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Sprintln("unable to get user home directory:", err))
	}
	return fmt.Sprintf("%s/.k3d/kubeconfig-%s.yaml", home, clusterName)
}

// RunCmdWithProgress runs a command and progressively outputs the progress.
func RunCmdWithProgress(cmd *exec.Cmd) ([]byte, error) {
	// Use pipes so we can output progress.
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	err := cmd.Start()
	if err != nil {
		fmt.Println("failed cmd:", cmd)
		panic(err)
	}

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
		fmt.Printf("[%s] invalid cluster id: %s\n", cn, idStr)
		os.Exit(1)
	}
	return id
}
