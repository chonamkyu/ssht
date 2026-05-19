package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/chonamkyu/ssht/internal/session"
	"github.com/spf13/cobra"
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Manage background sessions",
	Long: `List, attach, or kill background SSH sessions.

Examples:
  ssht sessions              # list all
  ssht sessions kill 1       # kill session 1
  ssht sessions kill --all   # kill all sessions`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return sessionsListRun()
	},
}

var sessionsKillCmd = &cobra.Command{
	Use:   "kill [session-id]",
	Short: "Kill a background session",
	RunE: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")

		if all {
			killed, err := session.KillAll()
			if err != nil {
				return err
			}
			fmt.Printf("Killed %d session(s).\n", killed)
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("specify a session ID or use --all")
		}

		id, err := parseSessionID(args[0])
		if err != nil {
			return err
		}

		if err := session.Kill(id); err != nil {
			return err
		}

		fmt.Printf("Session %d killed.\n", id)
		return nil
	},
}

var attachCmd = &cobra.Command{
	Use:   "attach <session-id>",
	Short: "Attach to a background session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parseSessionID(args[0])
		if err != nil {
			return err
		}
		return session.Attach(id)
	},
}

func init() {
	sessionsCmd.AddCommand(sessionsKillCmd)
	sessionsKillCmd.Flags().Bool("all", false, "Kill all sessions")
}

func sessionsListRun() error {
	sessions, err := session.List()
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Println("No active sessions.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tHOST\tSTATUS\tDURATION")
	for _, s := range sessions {
		duration := time.Since(s.StartedAt).Truncate(time.Second)
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", s.ID, s.HostName, s.Status, duration)
	}
	w.Flush()
	return nil
}

func parseSessionID(s string) (int, error) {
	var id int
	_, err := fmt.Sscanf(s, "%d", &id)
	if err != nil {
		return 0, fmt.Errorf("invalid session ID: %s", s)
	}
	return id, nil
}
