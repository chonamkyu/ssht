package cmd

import (
	"fmt"

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

Examples:
  ssht send 1 --cmd "ls -la"
  ssht send 1 --cmd "docker ps"
  ssht send 1 --key "ctrl+c"
  ssht send 1 --key "ctrl+d"
  ssht send 1 --key "up,up,enter"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parseSessionID(args[0])
		if err != nil {
			return err
		}

		cmdFlag, _ := cmd.Flags().GetString("cmd")
		keyFlag, _ := cmd.Flags().GetString("key")

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

		return session.SendInput(id, payload)
	},
}

func init() {
	sendCmd.Flags().StringP("cmd", "c", "", "Command to execute (auto-appends enter)")
	sendCmd.Flags().StringP("key", "k", "", "Key sequence to send (e.g., ctrl+c, up,enter)")
}
