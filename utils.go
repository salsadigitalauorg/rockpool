package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
)

func runCmdWithProgress(cmd *exec.Cmd) error {
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

func helmInstallOrUpgrade(releaseName string, chartName string, upgrade bool, args []string) error {
	cmd := exec.Command("helm")
	if upgrade {
		cmd.Args = append(cmd.Args, "upgrade", "--install")
	} else {
		cmd.Args = append(cmd.Args, "install")
	}
	cmd.Args = append(cmd.Args, releaseName, chartName)
	cmd.Args = append(cmd.Args, args...)
	fmt.Printf("command for %s helm release: %v\n", releaseName, cmd)
	return runCmdWithProgress(cmd)
}
