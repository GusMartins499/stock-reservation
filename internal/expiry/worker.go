package expiry

import (
	"context"
	"log/slog"
	"time"
)

type ExpireFunc func(ctx context.Context, limit int) (int, error)

type Worker struct {
	Name      string
	Expire    ExpireFunc
	Interval  time.Duration
	BatchSize int
	Logger    *slog.Logger
}

func (w Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	for {
		w.drain(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w Worker) drain(ctx context.Context) {
	for ctx.Err() == nil {
		n, err := w.Expire(ctx, w.BatchSize)
		if err != nil {
			if ctx.Err() == nil {
				w.Logger.Error("batch failed", "worker", w.Name, "err", err)
			}
			return
		}
		if n > 0 {
			w.Logger.Info("batch done", "worker", w.Name, "count", n)
		}
		if n < w.BatchSize {
			return
		}
	}
}
