package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/SadeepaNHerath/Spacefold/internal/engine"
	"github.com/SadeepaNHerath/Spacefold/internal/tui/status"
)

var (
	statusJSON     bool
	statusWatch    bool
	statusInterval time.Duration

	// isStdoutTTYFunc detects if stdout is attached to a terminal character device.
	// Can be mocked in unit tests.
	isStdoutTTYFunc = func() bool {
		info, err := os.Stdout.Stat()
		if err != nil {
			return false
		}
		return (info.Mode() & os.ModeCharDevice) != 0
	}
)

func init() {
	statusCmd.Run = func(cmd *cobra.Command, args []string) {
		_ = runStatus(cmd, args)
	}
	statusCmd.RunE = runStatus

	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output metrics in JSON format")
	statusCmd.Flags().BoolVarP(&statusWatch, "watch", "w", false, "Continuously stream system metrics")
	statusCmd.Flags().DurationVarP(&statusInterval, "interval", "i", time.Second, "Sampling interval for watch mode")
}

// runStatus handles the sf status command execution across TUI and headless/piped modes.
func runStatus(cmd *cobra.Command, args []string) error {
	jsonMode, _ := cmd.Flags().GetBool("json")
	watchMode, _ := cmd.Flags().GetBool("watch")
	interval, _ := cmd.Flags().GetDuration("interval")
	if interval <= 0 {
		interval = time.Second
	}

	isTTY := isStdoutTTYFunc()

	// 1. Non-interactive or JSON output mode (auto-detected pipe or explicit --json)
	if !isTTY || jsonMode {
		if watchMode {
			// Continuous stream: emit NDJSON (one line per JSON object)
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			// Emit initial sample immediately
			if err := outputSingleJSON(cmd, false); err != nil {
				return err
			}

			for {
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
					if err := outputSingleJSON(cmd, false); err != nil {
						return err
					}
				}
			}
		}

		// Single-shot execution: output formatted JSON
		return outputSingleJSON(cmd, true)
	}

	// 2. Interactive terminal mode: launch full Bubble Tea dashboard
	model := status.NewModel()
	model.Interval = interval

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// outputSingleJSON collects metrics and prints formatted or NDJSON to command output.
func outputSingleJSON(cmd *cobra.Command, indent bool) error {
	out := cmd.OutOrStdout()

	metrics, err := engine.CollectMetrics()
	if err != nil {
		return fmt.Errorf("failed to collect system metrics: %w", err)
	}

	var data []byte
	if indent {
		data, err = json.MarshalIndent(metrics, "", "  ")
	} else {
		data, err = json.Marshal(metrics)
	}

	if err != nil {
		return fmt.Errorf("failed to encode metrics JSON: %w", err)
	}

	_, writeErr := fmt.Fprintln(out, string(data))
	return writeErr
}
