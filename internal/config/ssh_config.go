package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func parseSSHConfig() []Host {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".ssh", "config")

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var hosts []Host
	var current *Host

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])

		switch key {
		case "host":
			if current != nil && current.Name != "" && !strings.Contains(current.Name, "*") {
				if current.Host == "" {
					current.Host = current.Name
				}
				if current.Port == 0 {
					current.Port = 22
				}
				hosts = append(hosts, *current)
			}
			current = &Host{
				Name:   val,
				Source: "ssh_config",
				Port:   22,
			}
		case "hostname":
			if current != nil {
				current.Host = val
			}
		case "user":
			if current != nil {
				current.User = val
			}
		case "port":
			if current != nil {
				if p, err := strconv.Atoi(val); err == nil {
					current.Port = p
				}
			}
		case "identityfile":
			if current != nil {
				if strings.HasPrefix(val, "~/") {
					home, _ := os.UserHomeDir()
					val = filepath.Join(home, val[2:])
				}
				current.Key = val
			}
		}
	}

	if current != nil && current.Name != "" && !strings.Contains(current.Name, "*") {
		if current.Host == "" {
			current.Host = current.Name
		}
		if current.Port == 0 {
			current.Port = 22
		}
		hosts = append(hosts, *current)
	}

	return hosts
}
