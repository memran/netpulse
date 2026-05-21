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
	} else if len(os.Args) > 1 && os.Args[1] != "run" && os.Args[1] != "--help" && os.Args[1] != "-h" && os.Args[1] != "--version" && os.Args[1] != "-v" && os.Args[1][0] != '-' {
		os.Args = append([]string{os.Args[0], "run"}, os.Args[1:]...)
	}

	app := console.New("netpulse").
		Version(version).
		Description("Terminal-based local internet diagnostic tool.")

	app.Register(&commands.RunCommand{})

	if err := app.Run(); err != nil {
		os.Exit(1)
	}
}
