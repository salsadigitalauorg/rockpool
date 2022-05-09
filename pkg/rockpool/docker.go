package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/yusufhm/rockpool/internal"
)

func (r *Rockpool) Docker(args ...string) *exec.Cmd {
	return exec.Command("docker", args...)
}

func (r *Rockpool) DockerExec(n string, cmdStr string) ([]byte, error) {
	return r.Docker("exec", n, "ash", "-c", cmdStr).Output()
}

func (r *Rockpool) DockerRestart(n string) ([]byte, error) {
	return r.Docker("restart", n).Output()
}

func (r *Rockpool) DockerInspect(cn string) []DockerContainer {
	cmd := r.Docker("inspect", cn)
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

func (r *Rockpool) DockerCp(src string, dest string) ([]byte, error) {
	return r.Docker("cp", src, dest).Output()
}
