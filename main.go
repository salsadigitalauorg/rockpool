package main

import (
	"github.com/salsadigitalauorg/rockpool/cmd"
)

// Version information.
var (
	version string
	commit  string
)

func main() {
	cmd.Version = version
	cmd.Commit = commit
	cmd.Execute()
}
