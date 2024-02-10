package resolver

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/salsadigitalauorg/rockpool/pkg/command"
	"github.com/salsadigitalauorg/rockpool/pkg/config"
	"github.com/salsadigitalauorg/rockpool/pkg/docker"

	log "github.com/sirupsen/logrus"
)

func Install() {
	nameserverIp := docker.GetVmIp()

	dest := filepath.Join("/etc/resolver", config.C.Hostname())
	logger := log.WithField("resolverFile", dest)
	logger.Info("installing resolver file")

	data := fmt.Sprintf(`
nameserver %s
port 6153
`, nameserverIp)

	var tmpFile *os.File
	var err error

	if _, err := os.Stat(dest); err == nil {
		logger.Debug("resolver file already exists")
		return
	}

	logger.Info("creating resolver file")
	if tmpFile, err = os.CreateTemp("", "rockpool-resolver-"); err != nil {
		logger.WithError(err).Panic("unable to create temporary file")
	}
	if err = os.Chmod(tmpFile.Name(), 0777); err != nil {
		logger.WithField("tempFile", tmpFile.Name()).WithError(err).
			Panic("unable to set file permissions")
	}
	if _, err = tmpFile.WriteString(data); err != nil {
		logger.WithField("tempFile", tmpFile.Name()).WithError(err).
			Panic("unable to write to temporary file")
	}
	if err = command.ShellCommander("sudo", "mv", tmpFile.Name(), dest).Run(); err != nil {
		logger.WithFields(log.Fields{
			"tempFile":    tmpFile.Name(),
			"destination": dest,
		}).WithError(command.GetMsgFromCommandError(err)).
			Panic("unable to move file")
	}
}

func Remove() {
	dest := filepath.Join("/etc/resolver", config.C.Hostname())
	logger := log.WithField("resolverFile", dest)
	logger.Info("removing resolver file")
	if err := command.ShellCommander("rm", "-f", dest).Run(); err != nil {
		logger.WithError(command.GetMsgFromCommandError(err)).
			Warn("error when deleting resolver file")
	}
}
