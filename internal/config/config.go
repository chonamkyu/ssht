package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Host struct {
	Name         string   `yaml:"name"`
	Host         string   `yaml:"host"`
	Port         int      `yaml:"port"`
	User         string   `yaml:"user"`
	Key          string   `yaml:"key,omitempty"`
	Password     string   `yaml:"password,omitempty"`
	PasswordAuth bool     `yaml:"password_auth,omitempty"`
	Group        string   `yaml:"group,omitempty"`
	Tags         []string `yaml:"tags,omitempty"`
	Source       string   `yaml:"-"`
}

type Config struct {
	Hosts []Host `yaml:"hosts"`
	path  string
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssht")
}

func ConfigPath() string {
	return filepath.Join(configDir(), "config.yaml")
}

func Load() (*Config, error) {
	dir := configDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	cfg := &Config{path: ConfigPath()}

	data, err := os.ReadFile(cfg.path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if len(data) > 0 {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
	}

	for i := range cfg.Hosts {
		if cfg.Hosts[i].Port == 0 {
			cfg.Hosts[i].Port = 22
		}
		cfg.Hosts[i].Source = "ssht"
		if cfg.Hosts[i].Password != "" {
			decrypted, err := decrypt(cfg.Hosts[i].Password)
			if err == nil {
				cfg.Hosts[i].Password = decrypted
			}
		}
	}

	sshHosts := parseSSHConfig()
	cfg.Hosts = append(cfg.Hosts, sshHosts...)

	return cfg, nil
}

func (c *Config) Save() error {
	toSave := &Config{}
	for _, h := range c.Hosts {
		if h.Source == "ssh_config" {
			continue
		}
		if h.Password != "" {
			encrypted, err := encrypt(h.Password)
			if err != nil {
				return fmt.Errorf("encrypting password for %s: %w", h.Name, err)
			}
			h.Password = encrypted
		}
		toSave.Hosts = append(toSave.Hosts, h)
	}

	data, err := yaml.Marshal(toSave)
	if err != nil {
		return err
	}

	return os.WriteFile(c.path, data, 0600)
}

func (c *Config) FindHost(name string) (*Host, error) {
	for i, h := range c.Hosts {
		if h.Name == name {
			return &c.Hosts[i], nil
		}
	}
	return nil, fmt.Errorf("host not found: %s", name)
}

func (c *Config) FindByTag(tag string) []Host {
	var result []Host
	for _, h := range c.Hosts {
		for _, t := range h.Tags {
			if t == tag {
				result = append(result, h)
				break
			}
		}
	}
	return result
}

func (c *Config) RenameHost(oldName, newName string) error {
	for i, h := range c.Hosts {
		if h.Name == oldName {
			if h.Source == "ssh_config" {
				return fmt.Errorf("cannot rename host from ~/.ssh/config (read-only)")
			}
			c.Hosts[i].Name = newName
			return nil
		}
	}
	return fmt.Errorf("host not found: %s", oldName)
}

func (c *Config) RemoveHost(name string) error {
	for i, h := range c.Hosts {
		if h.Name == name {
			if h.Source == "ssh_config" {
				return fmt.Errorf("cannot remove host from ~/.ssh/config (read-only)")
			}
			c.Hosts = append(c.Hosts[:i], c.Hosts[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("host not found: %s", name)
}
