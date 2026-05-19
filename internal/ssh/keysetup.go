package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chonamkyu/ssht/internal/config"
	gossh "golang.org/x/crypto/ssh"
)

func DefaultKeyPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "id_ed25519")
}

func KeyExists() bool {
	_, err := os.Stat(DefaultKeyPath())
	return err == nil
}

func DeployKey(host *config.Host, password string) error {
	pubKeyPath := DefaultKeyPath() + ".pub"
	pubKeyData, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return fmt.Errorf("reading public key: %w", err)
	}

	pubKeyStr := strings.TrimSpace(string(pubKeyData))

	cmd := fmt.Sprintf(`mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo "%s" >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys`, pubKeyStr)

	authMethods := []gossh.AuthMethod{gossh.Password(password)}

	user := host.User
	if user == "" {
		user = os.Getenv("USER")
	}

	cfg := &gossh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:%d", host.Host, host.Port)
	conn, err := gossh.Dial("tcp", addr, cfg)
	if err != nil {
		return fmt.Errorf("connecting for key deploy: %w", err)
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("deploying key: %w", err)
	}

	return nil
}

func GenerateKey() (string, error) {
	keyPath := DefaultKeyPath()

	if _, err := os.Stat(keyPath); err == nil {
		return keyPath, nil
	}

	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}

	privBytes, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(keyPath, pem.EncodeToMemory(privBytes), 0600); err != nil {
		return "", err
	}

	pub, err := gossh.NewPublicKey(priv.Public())
	if err != nil {
		return "", err
	}

	pubBytes := gossh.MarshalAuthorizedKey(pub)
	if err := os.WriteFile(keyPath+".pub", pubBytes, 0644); err != nil {
		return "", err
	}

	return keyPath, nil
}
