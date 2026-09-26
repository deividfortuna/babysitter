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
	}
	if err := next.Validate(); err != nil {
		return store.Settings{}, err
	}
	return next, nil
}
