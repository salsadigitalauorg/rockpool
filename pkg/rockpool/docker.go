package rockpool

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/yusufhm/rockpool/internal"
)

func (r *Rockpool) Docker(args ...string) *exec.Cmd {
	cmd := exec.Command("docker")
	cmd.Args = append(cmd.Args, args...)
	return cmd
}

func (r *Rockpool) DockerExec(n string, cmdStr string) ([]byte, error) {
	cmd := r.Docker("exec", n, "ash", "-c", cmdStr)
	return cmd.Output()
}

func (r *Rockpool) DockerInspect(cn string) []DockerContainer {
	cmd := r.Docker("inspect", cn)
	out, err := cmd.Output()
	if err != nil {
		fmt.Println(string(out))
		fmt.Println("unable to get list of docker containers: ", internal.GetCmdStdErr(err))
		os.Exit(1)
	}
	containers := []DockerContainer{}
	err = json.Unmarshal(out, &containers)
	if err != nil {
		fmt.Println("unable to parse docker containers: ", err)
		os.Exit(1)
	}
	return containers
}

func (r *Rockpool) DockerCp(src string, dest string) ([]byte, error) {
	cmd := r.Docker("cp", src, dest)
	return cmd.Output()
}