package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chonamkyu/ssht/internal/config"
	"github.com/chonamkyu/ssht/internal/multi"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <command>",
	Short: "Execute a command on multiple hosts in parallel",
	Long: `Execute a command on multiple hosts simultaneously.

Examples:
  ssht exec "uptime" -h prod-web-1,prod-web-2
  ssht exec "df -h" --tag prod
  ssht exec "systemctl status nginx" --tag web --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		hostsFlag, _ := cmd.Flags().GetString("hosts")
		tagFlag, _ := cmd.Flags().GetString("tag")
		jsonFlag, _ := cmd.Flags().GetBool("json")

		var targets []config.Host

		if hostsFlag != "" {
			names := strings.Split(hostsFlag, ",")
			for _, name := range names {
				h, err := cfg.FindHost(strings.TrimSpace(name))
				if err != nil {
					return err
				}
				targets = append(targets, *h)
			}
		}

		if tagFlag != "" {
			tagged := cfg.FindByTag(tagFlag)
			targets = append(targets, tagged...)
		}

		if len(targets) == 0 {
			return fmt.Errorf("no target hosts specified. Use -h or --tag")
		}

		results := multi.Execute(targets, args[0])

		if jsonFlag {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(results)
		}

		for _, r := range results {
			fmt.Printf("━━━ %s ━━━\n", r.Host)
			if r.Error != "" {
				fmt.Printf("ERROR: %s\n", r.Error)
			} else {
				fmt.Print(r.Output)
			}
			fmt.Println()
		}
		return nil
	},
}

func init() {
	execCmd.Flags().StringP("hosts", "h", "", "Comma-separated list of host names")
	execCmd.Flags().StringP("tag", "t", "", "Execute on all hosts with this tag")
	execCmd.Flags().Bool("json", false, "Output results as JSON")
}
