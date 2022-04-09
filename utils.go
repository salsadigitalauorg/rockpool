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

func helmAddRepo(name string, url string) error {
	cmd := exec.Command("helm", "repo", "add", name, url)
	return runCmdWithProgress(cmd)
}

func helmInstallOrUpgrade(releaseName string, chartName string, args []string) error {
	for _, r := range helmReleases {
		if r.Name == releaseName {
			fmt.Printf("helm release %s is already installed\n", releaseName)
			return nil
		}
	}

	cmd := exec.Command("helm", "--kubeconfig", kubeconfig)
	cmd.Args = append(cmd.Args, "install")
	cmd.Args = append(cmd.Args, releaseName, chartName)
	cmd.Args = append(cmd.Args, args...)
	fmt.Printf("command for %s helm release: %v\n", releaseName, cmd)
	return runCmdWithProgress(cmd)
}
