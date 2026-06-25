package pricing

import (
	"context"
	"log/slog"
)

// Bot updates worker contract price configuration.
type Bot struct {
	logger *slog.Logger
}

// New creates a price bot.
func New(logger *slog.Logger) *Bot {
	return &Bot{logger: logger}
}

// Run starts the price update loop until the context is canceled.
func (b *Bot) Run(ctx context.Context) error {
	b.logger.Info("price bot loop started")
	<-ctx.Done()
	return ctx.Err()
}
