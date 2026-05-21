package commands

import (
	"os"

	"go-consolekit/console"

	"github.com/memran/netpulse/internal/config"
)

type ConfigInitCommand struct{}

func (c *ConfigInitCommand) Name() string {
	return "config:init"
}

func (c *ConfigInitCommand) Description() string {
	return "Create a default settings.yml file."
}

func (c *ConfigInitCommand) Configure(cfg *console.CommandConfig) {
	cfg.Argument("path").Default("settings.yml").Description("Output path for the config file")
}

func (c *ConfigInitCommand) Handle(ctx *console.Context) error {
	path := ctx.Arg("path")
	if _, err := os.Stat(path); err == nil {
		ctx.Warning("Config file already exists: " + path)
		return nil
	}
	if err := config.WriteDefaultFile(path); err != nil {
		return err
	}
	ctx.Success("Created config: " + path)
	return nil
}
