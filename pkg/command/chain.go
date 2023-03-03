package command

import log "github.com/sirupsen/logrus"

type ChainCommand struct {
	Stage   string
	Command IShellCommand
}

type Chain struct {
	Commands []ChainCommand
}

func (c *Chain) Add(stage string, cmd IShellCommand) *Chain {
	c.Commands = append(c.Commands, ChainCommand{Stage: stage, Command: cmd})
	return c
}

func (c *Chain) Exec() error {
	for _, cc := range c.Commands {
		log.Infof("[%s] running command '%s'\n", cc.Command)
		err := cc.Command.RunProgressive()
		if err != nil {
			return err
		}
	}
	return nil
}
