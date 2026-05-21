package commands

import (
	"go-consolekit/console"

	"github.com/memran/netpulse/internal/app"
)

type RunCommand struct{}

func (c *RunCommand) Name() string {
	return "run"
}

func (c *RunCommand) Description() string {
	return "Run the NetPulse dashboard."
}

func (c *RunCommand) Configure(cfg *console.CommandConfig) {
	cfg.Argument("config_path").Default("").Description("Optional path to a settings.yml file or config directory")
}

func (c *RunCommand) Handle(ctx *console.Context) error {
	return app.Run(ctx.Arg("config_path"))
}
