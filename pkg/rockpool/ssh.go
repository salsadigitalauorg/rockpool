package rockpool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	"golang.org/x/crypto/ssh"
)

func (r *Rockpool) SshGetPublicKey() []byte {
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
		fmt.Println("[rockpool] error reading ssh key:", err)
		os.Exit(1)
	}
	return data
}

func (r *Rockpool) SshGetPublicKeyFingerprint() (string, string, string, string) {
	key := r.SshGetPublicKey()
	pk, comment, _, _, err := ssh.ParseAuthorizedKey(key)
	if err != nil {
		panic(err)
	}
	return strings.Split(string(key), " ")[1], pk.Type(), ssh.FingerprintSHA256(pk), comment
}
