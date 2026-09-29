package daemon

import (
	"github.com/deividfortuna/babysitter/internal/store"
)

func runningSettings(stored store.Settings, cfg Config) (store.Settings, error) {
	next := stored
	if cfg.Interval != 0 {
		next.PollInterval = cfg.Interval
	}
	if cfg.WatchInterval != 0 {
		next.WatchInterval = cfg.WatchInterval
		next.WatchMaxInterval = max(next.WatchMaxInterval, next.WatchInterval)
	}
	if cfg.WatchMaxInterval != 0 {
		next.WatchMaxInterval = cfg.WatchMaxInterval
	}
	if cfg.CheckMaxInterval != 0 {
		next.CheckMaxInterval = cfg.CheckMaxInterval
	}
	if err := next.Validate(); err != nil {
		return store.Settings{}, err
	}
	return next, nil
}

func intervalsOverridden(running, stored store.Settings) bool {
	return running.PollInterval != stored.PollInterval ||
		running.WatchInterval != stored.WatchInterval ||
		running.WatchMaxInterval != stored.WatchMaxInterval ||
		running.CheckMaxInterval != stored.CheckMaxInterval
}
