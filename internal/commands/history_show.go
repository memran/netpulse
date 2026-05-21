package commands

import (
	"fmt"

	"go-consolekit/console"

	"github.com/memran/netpulse/internal/config"
	"github.com/memran/netpulse/internal/storage"
)

type HistoryShowCommand struct{}

func (c *HistoryShowCommand) Name() string {
	return "history:show"
}

func (c *HistoryShowCommand) Description() string {
	return "Show recent history summary from the SQLite database."
}

func (c *HistoryShowCommand) Configure(cfg *console.CommandConfig) {
	cfg.Argument("config_path").Default("").Description("Optional path to a settings.yml file or config directory")
}

func (c *HistoryShowCommand) Handle(ctx *console.Context) error {
	cfg, err := config.Load(ctx.Arg("config_path"))
	if err != nil {
		return err
	}

	repo, err := storage.New(cfg.Storage.SQLitePath)
	if err != nil {
		return err
	}
	defer repo.Close()

	rows, err := repo.WeeklyHistorySummary()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		ctx.Info("No history available")
		return nil
	}

	table := ctx.Output().Table().
		Headers("Date", "Uptime", "Avg Latency", "Packet Loss", "Downtime")
	for _, row := range rows {
		table.Row(
			row.Date,
			row.UptimeStr,
			fmt.Sprintf("%.1f ms", row.AvgLatency),
			fmt.Sprintf("%.1f %%", row.PacketLoss),
			row.DowntimeStr,
		)
	}
	table.Render()
	return nil
}
