package main

import (
	"os"

	"go-consolekit/console"

	"github.com/memran/netpulse/internal/commands"
)

const version = "0.1.0"

func main() {
	// Preserve the current UX: `netpulse [config-path]` maps to `netpulse run [config-path]`.
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "run")
	} else if len(os.Args) > 1 && shouldDefaultToRun(os.Args[1]) {
		os.Args = append([]string{os.Args[0], "run"}, os.Args[1:]...)
	}

	app := console.New("netpulse").
		Version(version).
		Description("Terminal-based local internet diagnostic tool.")

	app.Register(
		&commands.RunCommand{},
		&commands.ConfigInitCommand{},
		&commands.ConfigValidateCommand{},
		&commands.HistoryShowCommand{},
	)

	if err := app.Run(); err != nil {
		os.Exit(1)
	}
}

func shouldDefaultToRun(arg string) bool {
	if arg == "" || arg[0] == '-' {
		return false
	}
	switch arg {
	case "run", "config", "history", "help", "completion":
		return false
	default:
		return true
	}
}
