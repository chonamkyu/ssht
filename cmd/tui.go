package cmd

import (
	"fmt"

	"github.com/chonamkyu/ssht/internal/tui"
)

func runTUI() error {
	if err := tui.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}
