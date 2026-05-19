package cmd

import (
	"fmt"
	"time"

	"github.com/chonamkyu/ssht/internal/keys"
	"github.com/chonamkyu/ssht/internal/session"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <session-id>",
	Short: "Send command or keystrokes to a background session",
	Long: `Send a command or keystrokes to a running background session.

--cmd sends text with a trailing newline (executes the command).
--key sends control keys or key sequences.
--output reads N lines from the session after sending (like tail).

Examples:
  ssht send 1 --cmd "ls -la"
  ssht send 1 --cmd "ls -la" --output 20
  ssht send 1 --cmd "docker ps" -o 50
  ssht send 1 --key "ctrl+c"
  ssht send 1 --key "up,up,enter" -o 10`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parseSessionID(args[0])
		if err != nil {
			return err
		}

		cmdFlag, _ := cmd.Flags().GetString("cmd")
		keyFlag, _ := cmd.Flags().GetString("key")
		output, _ := cmd.Flags().GetInt("output")

		if cmdFlag == "" && keyFlag == "" {
			return fmt.Errorf("must specify --cmd or --key")
		}

		var payload []byte

		if cmdFlag != "" {
			payload = append(payload, []byte(cmdFlag)...)
			payload = append(payload, '\n')
		}

		if keyFlag != "" {
			parsed, err := keys.ParseKeys(keyFlag)
			if err != nil {
				return err
			}
			payload = append(payload, parsed...)
		}

		if err := session.SendInput(id, payload); err != nil {
			return err
		}

		if output > 0 {
			time.Sleep(500 * time.Millisecond)
			out, err := session.ReadOutput(id, output)
			if err != nil {
				return err
			}
			fmt.Print(out)
		}

		return nil
	},
}

func init() {
	sendCmd.Flags().StringP("cmd", "c", "", "Command to execute (auto-appends enter)")
	sendCmd.Flags().StringP("key", "k", "", "Key sequence to send (e.g., ctrl+c, up,enter)")
	sendCmd.Flags().IntP("output", "o", 0, "Read N lines from session after sending")
}
