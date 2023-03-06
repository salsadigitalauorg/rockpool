package ssh

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/salsadigitalauorg/rockpool/pkg/platform"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

func GetPublicKey() []byte {
	home, _ := os.UserHomeDir()
	var keyFile string
	idEd25519 := filepath.Join(home, ".ssh", "id_ed25519.pub")
	idRsa := filepath.Join(home, ".ssh", "id_rsa.pub")
	if platform.LagoonSshKey != "" {
		keyFile = platform.LagoonSshKey
	} else if _, err := os.Stat(idEd25519); err == nil {
		keyFile = idEd25519
	} else if _, err := os.Stat(idRsa); err == nil {
		keyFile = idRsa
	}
	data, err := os.ReadFile(keyFile)
	if err != nil {
		log.WithField("keyFile", keyFile).WithError(err).Fatal("error reading ssh key")
	}
	return data
}

func GetPublicKeyFingerprint() (string, string, string, string) {
	key := GetPublicKey()
	pk, comment, _, _, err := ssh.ParseAuthorizedKey(key)
	if err != nil {
		panic(err)
	}
	return strings.Split(string(key), " ")[1], pk.Type(), ssh.FingerprintSHA256(pk), comment
}
