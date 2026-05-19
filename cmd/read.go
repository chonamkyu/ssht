package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/chonamkyu/ssht/internal/session"
	"github.com/spf13/cobra"
)

var readCmd = &cobra.Command{
	Use:   "read <session-id>",
	Short: "Read terminal output from a background session",
	Long: `Read the terminal output buffer from a background session.

Examples:
  ssht read 1            # read current buffer
  ssht read 1 --follow   # stream output continuously
  ssht read 1 --lines 50 # read last 50 lines`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := parseSessionID(args[0])
		if err != nil {
			return err
		}

		follow, _ := cmd.Flags().GetBool("follow")
		lines, _ := cmd.Flags().GetInt("lines")

		if follow {
			reader, err := session.Follow(id)
			if err != nil {
				return err
			}
			_, err = io.Copy(os.Stdout, reader)
			return err
		}

		output, err := session.ReadOutput(id, lines)
		if err != nil {
			return err
		}

		fmt.Print(output)
		return nil
	},
}

func init() {
	readCmd.Flags().BoolP("follow", "f", false, "Stream output continuously")
	readCmd.Flags().IntP("lines", "n", 100, "Number of lines to read from buffer")
}
