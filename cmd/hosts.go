package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/chonamkyu/ssht/internal/config"
	"github.com/spf13/cobra"
)

var hostsCmd = &cobra.Command{
	Use:   "hosts",
	Short: "Manage SSH hosts",
	RunE: func(cmd *cobra.Command, args []string) error {
		return hostsListRun()
	},
}

var hostsAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new host",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		host, _ := cmd.Flags().GetString("host")
		user, _ := cmd.Flags().GetString("user")
		port, _ := cmd.Flags().GetInt("port")
		key, _ := cmd.Flags().GetString("key")
		tags, _ := cmd.Flags().GetStringSlice("tags")

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		h := config.Host{
			Name: name,
			Host: host,
			User: user,
			Port: port,
			Key:  key,
			Tags: tags,
		}

		cfg.Hosts = append(cfg.Hosts, h)
		if err := cfg.Save(); err != nil {
			return err
		}

		fmt.Printf("Host '%s' added.\n", name)
		return nil
	},
}

var hostsRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a host",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if err := cfg.RemoveHost(args[0]); err != nil {
			return err
		}

		if err := cfg.Save(); err != nil {
			return err
		}

		fmt.Printf("Host '%s' removed.\n", args[0])
		return nil
	},
}

func init() {
	hostsCmd.AddCommand(hostsAddCmd)
	hostsCmd.AddCommand(hostsRemoveCmd)

	hostsAddCmd.Flags().String("name", "", "Host alias name")
	hostsAddCmd.Flags().String("host", "", "Hostname or IP")
	hostsAddCmd.Flags().String("user", "", "SSH user")
	hostsAddCmd.Flags().Int("port", 22, "SSH port")
	hostsAddCmd.Flags().String("key", "", "Path to SSH private key")
	hostsAddCmd.Flags().StringSlice("tags", nil, "Tags for grouping")
	hostsAddCmd.MarkFlagRequired("name")
	hostsAddCmd.MarkFlagRequired("host")
}

func hostsListRun() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if len(cfg.Hosts) == 0 {
		fmt.Println("No hosts configured. Use 'ssht hosts add' or add hosts to ~/.ssh/config")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tHOST\tUSER\tPORT\tSOURCE\tTAGS")
	for _, h := range cfg.Hosts {
		source := "ssht"
		if h.Source == "ssh_config" {
			source = "~/.ssh/config"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%v\n", h.Name, h.Host, h.User, h.Port, source, h.Tags)
	}
	w.Flush()
	return nil
}
