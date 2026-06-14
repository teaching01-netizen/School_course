package emailreminder

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Config struct {
	Enabled bool
	Time    string
}

type Scheduler struct {
	log         *slog.Logger
	cfg         Config
	instituteTZ string
	sendAll     func(ctx context.Context) error

	mu          sync.Mutex
	lastRunDate string
	stopped     chan struct{}
}

func New(log *slog.Logger, cfg Config, instituteTZ string, sendAll func(ctx context.Context) error) *Scheduler {
	if cfg.Time == "" {
		cfg.Time = "08:00"
	}
	return &Scheduler{
		log:         log,
		cfg:         cfg,
		instituteTZ: instituteTZ,
		sendAll:     sendAll,
		stopped:     make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	if !s.cfg.Enabled {
		s.log.Info("email reminder scheduler disabled, set EMAIL_REMINDER_ENABLED=true to enable")
		return
	}
	s.log.Info("email reminder scheduler enabled", "time", s.cfg.Time, "tz", s.instituteTZ)
	go s.loop(ctx)
}

func (s *Scheduler) Stop() {
	close(s.stopped)
}

func (s *Scheduler) loop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopped:
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	s.tickAt(ctx, nowInTZ(s.instituteTZ))
}

func (s *Scheduler) tickAt(ctx context.Context, now time.Time) {
	targetHour, targetMin, err := parseTime(s.cfg.Time)
	if err != nil {
		s.log.Error("email reminder: invalid time config", "time", s.cfg.Time, "error", err)
		return
	}

	if now.Hour() != targetHour || now.Minute() != targetMin {
		return
	}

	today := now.Format("2006-01-02")

	s.mu.Lock()
	if s.lastRunDate == today {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	s.log.Info("email reminder: triggering send for today")
	if err := s.sendAll(ctx); err != nil {
		s.log.Error("email reminder: send failed", "error", err)
		return
	}

	s.mu.Lock()
	s.lastRunDate = today
	s.mu.Unlock()
	s.log.Info("email reminder: send completed for today", "date", today)
}

func parseTime(t string) (hour, min int, err error) {
	parsed, err := time.Parse("15:04", t)
	if err != nil {
		return 0, 0, err
	}
	return parsed.Hour(), parsed.Minute(), nil
}

func nowInTZ(tz string) time.Time {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Now().UTC()
	}
	return time.Now().In(loc)
}
