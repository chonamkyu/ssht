package session

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/chonamkyu/ssht/internal/config"
)

const daemonEnvKey = "SSHT_DAEMON_MODE"

func IsDaemon() bool {
	return os.Getenv(daemonEnvKey) != ""
}

func SpawnDaemon(host *config.Host) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}

	args := []string{"connect", host.Name, "--background"}

	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), daemonEnvKey+"=1")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("spawning daemon: %w", err)
	}

	cmd.Process.Release()

	// Wait briefly for session info file to appear, then read ID
	pidFile := fmt.Sprintf("%s/daemon-%d.pid", sessionsDir(), cmd.Process.Pid)
	os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0600)

	return cmd.Process.Pid, nil
}

func RunDaemon(host *config.Host) {
	id, err := StartBackground(host)
	if err != nil {
		os.Exit(1)
	}

	_ = id

	// Block forever — session goroutines keep running
	s := getSession(id)
	if s != nil {
		<-s.done
	}
}
