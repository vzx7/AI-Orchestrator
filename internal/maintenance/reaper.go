// Package maintenance provides background maintenance loops for the orchestrator.
//
// V5 adds:
// - Visibility timeout reaper
// - Graceful shutdown support
package maintenance

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"ai_orchestrator/internal/queue"
)

type ReaperConfig struct {
	Interval time.Duration
	Timeout  time.Duration
}

func DefaultReaperConfig() ReaperConfig {
	return ReaperConfig{
		Interval: 10 * time.Second,
		Timeout:  60 * time.Second,
	}
}

type VisibilityReaper struct {
	logger  *slog.Logger
	queue   *queue.MemoryQueue
	config  ReaperConfig
	stopCh  chan struct{}
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex
}

func NewVisibilityReaper(logger *slog.Logger, q *queue.MemoryQueue, cfg ReaperConfig) *VisibilityReaper {
	return &VisibilityReaper{
		logger: logger,
		queue:  q,
		config: cfg,
		stopCh: make(chan struct{}),
	}
}

func (vr *VisibilityReaper) Start(ctx context.Context) {
	vr.mu.Lock()
	if vr.running {
		vr.mu.Unlock()
		return
	}
	vr.running = true
	vr.mu.Unlock()

	vr.wg.Add(1)
	go vr.run(ctx)

	vr.logger.Info("Visibility reaper started",
		"interval", vr.config.Interval,
		"timeout", vr.config.Timeout,
	)
}

func (vr *VisibilityReaper) run(ctx context.Context) {
	defer vr.wg.Done()

	ticker := time.NewTicker(vr.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			vr.logger.Info("Visibility reaper stopping due to context")
			return
		case <-vr.stopCh:
			vr.logger.Info("Visibility reaper stopped")
			return
		case <-ticker.C:
			reaped := vr.queue.ReapTimedOutTasks()
			if reaped > 0 {
				vr.logger.Info("Reaped timed-out tasks",
					"count", reaped,
					"timeout", vr.config.Timeout,
				)
			}
		}
	}
}

func (vr *VisibilityReaper) Stop() {
	vr.mu.Lock()
	if !vr.running {
		vr.mu.Unlock()
		return
	}
	vr.running = false
	vr.mu.Unlock()

	close(vr.stopCh)
	vr.wg.Wait()

	vr.logger.Info("Visibility reaper stopped")
}

func (vr *VisibilityReaper) IsRunning() bool {
	vr.mu.RLock()
	defer vr.mu.RUnlock()
	return vr.running
}
