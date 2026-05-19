package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ssht",
	Short: "Interactive SSH session manager with AI control",
	Long:  "ssht is an interactive SSH CLI tool that supports background sessions, AI keystroke control, and multi-server execution.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(hostsCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(attachCmd)
	rootCmd.AddCommand(sendCmd)
	rootCmd.AddCommand(readCmd)
	rootCmd.AddCommand(execCmd)
}
