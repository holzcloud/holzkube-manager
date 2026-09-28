package host

import (
	"context"
	"io/fs"
	"time"
)

// Config is what a Collector reads through.
type Config struct {
	FS  fs.FS
	Sys Sys
	Now func() time.Time
}

// Collector reads the host.
type Collector struct {
	cfg Config
}

// New builds a Collector.
func New(cfg Config) *Collector {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Collector{cfg: cfg}
}

// Read is not implemented yet (RED).
func (c *Collector) Read(context.Context) View {
	return View{}
}
