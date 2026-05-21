package commands

import (
	"go-consolekit/console"

	"github.com/memran/netpulse/internal/config"
)

type ConfigValidateCommand struct{}

func (c *ConfigValidateCommand) Name() string {
	return "config:validate"
}

func (c *ConfigValidateCommand) Description() string {
	return "Validate a NetPulse YAML configuration file."
}

func (c *ConfigValidateCommand) Configure(cfg *console.CommandConfig) {
	cfg.Argument("config_path").Default("").Description("Optional path to a settings.yml file or config directory")
}

func (c *ConfigValidateCommand) Handle(ctx *console.Context) error {
	cfg, err := config.Load(ctx.Arg("config_path"))
	if err != nil {
		return err
	}
	ctx.Success("Configuration is valid")
	ctx.Line("SQLite path: " + cfg.Storage.SQLitePath)
	return nil
}
