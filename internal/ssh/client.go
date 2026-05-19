package ssh

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/chonamkyu/ssht/internal/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type Client struct {
	conn    *ssh.Client
	host    *config.Host
	session *ssh.Session
}

func Connect(host *config.Host) (*Client, error) {
	authMethods, err := buildAuthMethods(host)
	if err != nil {
		return nil, err
	}

	user := host.User
	if user == "" {
		user = os.Getenv("USER")
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(host.Host, fmt.Sprintf("%d", host.Port))
	conn, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", host.Name, err)
	}

	return &Client{conn: conn, host: host}, nil
}

func (c *Client) Close() error {
	if c.session != nil {
		c.session.Close()
	}
	return c.conn.Close()
}

func (c *Client) Execute(command string) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	return string(output), err
}

func (c *Client) Shell(stdin io.Reader, stdout, stderr io.Writer) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return err
	}
	c.session = session

	session.Stdin = stdin
	session.Stdout = stdout
	session.Stderr = stderr

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		return fmt.Errorf("requesting PTY: %w", err)
	}

	if err := session.Shell(); err != nil {
		return fmt.Errorf("starting shell: %w", err)
	}

	return session.Wait()
}

func (c *Client) StartShell() (*ssh.Session, io.WriteCloser, io.Reader, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return nil, nil, nil, err
	}
	c.session = session

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		session.Close()
		return nil, nil, nil, err
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, nil, nil, err
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, nil, nil, err
	}

	if err := session.Shell(); err != nil {
		session.Close()
		return nil, nil, nil, err
	}

	return session, stdin, stdout, nil
}

func TestPassword(host *config.Host, password string) error {
	user := host.User
	if user == "" {
		user = os.Getenv("USER")
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	addr := net.JoinHostPort(host.Host, fmt.Sprintf("%d", host.Port))
	conn, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

func buildAuthMethods(host *config.Host) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if host.Key != "" {
		key, err := os.ReadFile(host.Key)
		if err != nil {
			return nil, fmt.Errorf("reading key %s: %w", host.Key, err)
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parsing key %s: %w", host.Key, err)
		}

		methods = append(methods, ssh.PublicKeys(signer))
	}

	defaultKeys := []string{
		os.Getenv("HOME") + "/.ssh/id_ed25519",
		os.Getenv("HOME") + "/.ssh/id_rsa",
	}

	for _, keyPath := range defaultKeys {
		key, err := os.ReadFile(keyPath)
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			continue
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if host.Password != "" {
		methods = append(methods, ssh.Password(host.Password))
	} else {
		methods = append(methods, ssh.PasswordCallback(func() (string, error) {
			fmt.Printf("Password for %s@%s: ", host.User, host.Host)
			pw, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return "", err
			}
			return string(pw), nil
		}))
	}

	methods = append(methods, ssh.KeyboardInteractive(
		func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i, q := range questions {
				fmt.Printf("%s", q)
				if echos[i] {
					var ans string
					fmt.Scanln(&ans)
					answers[i] = ans
				} else {
					pw, err := term.ReadPassword(int(os.Stdin.Fd()))
					fmt.Println()
					if err != nil {
						return nil, err
					}
					answers[i] = string(pw)
				}
			}
			return answers, nil
		},
	))

	return methods, nil
}
