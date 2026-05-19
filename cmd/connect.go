package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/chonamkyu/ssht/internal/config"
	"github.com/chonamkyu/ssht/internal/session"
	sshclient "github.com/chonamkyu/ssht/internal/ssh"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var connectCmd = &cobra.Command{
	Use:   "connect <host>",
	Short: "Connect to a host (auto-saves to profile)",
	Long: `Connect to an SSH host. Accepts saved host names or direct addresses.

If the host is not in your profile, it will be saved automatically.
On first connection, offers to deploy your SSH key for passwordless login.

Examples:
  ssht connect myserver
  ssht connect user@10.0.1.5
  ssht connect root@example.com:2222
  ssht connect example.com -b
  ssht connect example.com -p "password"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		password, _ := cmd.Flags().GetString("password")
		isNew := false

		host, err := cfg.FindHost(args[0])
		if err != nil {
			host, err = parseAndSaveHost(cfg, args[0])
			if err != nil {
				return err
			}
			isNew = true
		}

		if password != "" {
			host.Password = password
			cfg.Save()
		}

		if isNew {
			fmt.Printf("\n  [+] New host registered: %s\n", host.Host)
			fmt.Printf("      User: %s, Port: %d\n\n", host.User, host.Port)

			if err := offerKeySetup(cfg, host); err != nil {
				fmt.Printf("  [!] Key setup skipped: %v\n\n", err)
			}

			resolveHostname(host, cfg)
			fmt.Printf("  [+] Hostname resolved: %s\n\n", host.Name)
		}

		bg, _ := cmd.Flags().GetBool("background")
		if bg {
			if session.IsDaemon() {
				session.RunDaemon(host)
				return nil
			}
			pid, err := session.SpawnDaemon(host)
			if err != nil {
				return err
			}
			fmt.Printf("Session started in background: %s (pid: %d)\n", host.Name, pid)
			return nil
		}

		return session.StartInteractive(host)
	},
}

func init() {
	connectCmd.Flags().BoolP("background", "b", false, "Start session in background")
	connectCmd.Flags().StringP("password", "p", "", "SSH password")
}

func parseAndSaveHost(cfg *config.Config, addr string) (*config.Host, error) {
	h := config.Host{Port: 22}

	if at := strings.Index(addr, "@"); at != -1 {
		h.User = addr[:at]
		addr = addr[at+1:]
	}

	if colon := strings.LastIndex(addr, ":"); colon != -1 {
		port, err := strconv.Atoi(addr[colon+1:])
		if err != nil {
			return nil, fmt.Errorf("invalid port: %s", addr[colon+1:])
		}
		h.Port = port
		addr = addr[:colon]
	}

	h.Host = addr
	h.Name = h.Host
	if h.User == "" {
		h.User = os.Getenv("USER")
	}

	cfg.Hosts = append(cfg.Hosts, h)
	if err := cfg.Save(); err != nil {
		return nil, err
	}

	return &cfg.Hosts[len(cfg.Hosts)-1], nil
}

func resolveHostname(host *config.Host, cfg *config.Config) {
	client, err := sshclient.Connect(host)
	if err != nil {
		return
	}
	defer client.Close()

	output, err := client.Execute("hostname")
	if err != nil {
		return
	}

	name := strings.TrimSpace(output)
	if name != "" && name != host.Name {
		host.Name = name
		cfg.Save()
	}
}

func offerKeySetup(cfg *config.Config, host *config.Host) error {
	if host.Key != "" {
		return nil
	}

	if sshclient.KeyExists() {
		fmt.Printf("  [?] SSH key found at %s\n", sshclient.DefaultKeyPath())
		fmt.Printf("      Deploy to %s for passwordless login? [Y/n]: ", host.Name)
	} else {
		fmt.Printf("  [?] No SSH key found. Generate one and deploy to %s? [Y/n]: ", host.Name)
	}

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "" && answer != "y" && answer != "yes" {
		return nil
	}

	if !sshclient.KeyExists() {
		fmt.Print("  [*] Generating ed25519 key...")
		keyPath, err := sshclient.GenerateKey()
		if err != nil {
			return err
		}
		fmt.Printf(" done (%s)\n", keyPath)
	}

	pw := host.Password
	if pw == "" {
		var err error
		pw, err = promptPasswordWithRetry(host)
		if err != nil {
			return err
		}
		host.Password = pw
		cfg.Save()
	}

	fmt.Print("  [*] Deploying public key...")
	if err := sshclient.DeployKey(host, pw); err != nil {
		return err
	}
	fmt.Println(" done!")

	host.Key = sshclient.DefaultKeyPath()
	cfg.Save()

	fmt.Printf("  [+] Key deployed! Future connections will use key auth.\n\n")
	return nil
}

func promptPasswordWithRetry(host *config.Host) (string, error) {
	for attempt := 1; attempt <= 3; attempt++ {
		fmt.Printf("  [*] Password for %s@%s: ", host.User, host.Host)
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}

		password := string(pw)
		if err := sshclient.TestPassword(host, password); err != nil {
			if attempt < 3 {
				fmt.Printf("  [!] Authentication failed. Retry (%d/3)\n", attempt)
			} else {
				return "", fmt.Errorf("authentication failed after 3 attempts")
			}
			continue
		}

		return password, nil
	}
	return "", fmt.Errorf("authentication failed")
}
