package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/salsadigitalauorg/rockpool/internal"
)

func Exec(n string, cmdStr string) ([]byte, error) {
	return exec.Command("docker", "exec", n, "ash", "-c", cmdStr).Output()
}

func Stop(n string) ([]byte, error) {
	return exec.Command("docker", "stop", n).Output()
}

func Start(n string) ([]byte, error) {
	return exec.Command("docker", "start", n).Output()
}

func Restart(n string) ([]byte, error) {
	return exec.Command("docker", "restart", n).Output()
}

func Inspect(cn string) []DockerContainer {
	cmd := exec.Command("docker", "inspect", cn)
	out, err := cmd.Output()
	if err != nil {
		fmt.Println(string(out))
		fmt.Printf("[%s] unable to get list of docker containers: %s\n", cn, internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	containers := []DockerContainer{}
	err = json.Unmarshal(out, &containers)
	if err != nil {
		fmt.Printf("[%s] unable to parse docker containers: %s\n", cn, err)
		os.Exit(1)
	}
	return containers
}

func Cp(src string, dest string) ([]byte, error) {
	return exec.Command("docker", "cp", src, dest).Output()
}
