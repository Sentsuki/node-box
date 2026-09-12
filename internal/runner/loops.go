package runner

import (
	"context"
	"time"

	"node-box/internal/control"
	"node-box/internal/logx"
	"node-box/internal/model"
	"node-box/internal/source"
)

// bootstrapScheduleDelay is used before the first snapshot has been read, when
// the configured schedule is not known yet.
const bootstrapScheduleDelay = time.Hour

// scheduleLoop re-fetches subscriptions on the configured period.
//
// This is a plain loop that recomputes its delay each iteration, so a changed
// update_schedule takes effect on the next tick with no restarting, no
// recursion and no stale timers.
func (r *Runner) scheduleLoop(ctx context.Context) {
	for {
		delay := nextScheduleDelay(r.currentSchedule(), time.Now())

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
			r.Trigger(control.Trigger{Kind: control.KindSchedule})
		}
	}
}

// nextScheduleDelay returns how long to wait before the next scheduled update.
func nextScheduleDelay(s *model.Schedule, now time.Time) time.Duration {
	if s == nil {
		return bootstrapScheduleDelay
	}
	switch s.Type {
	case model.ScheduleHourly:
		return now.Truncate(time.Hour).Add(time.Hour).Sub(now)
	case model.ScheduleInterval:
		if d := s.Every.Duration(); d > 0 {
			return d
		}
	}
	return bootstrapScheduleDelay
}

// pollLoop watches the source for changes as a backstop for the webhook.
//
// Webhook deliveries get lost: the process may be restarting, the network may
// drop, the workflow may time out. A conditional request that answers 304 is
// cheap and does not count against the GitHub rate limit, so reconciling every
// few minutes costs almost nothing and removes the single point of failure.
func (r *Runner) pollLoop(ctx context.Context) {
	interval := r.boot.Source.PollInterval.Duration()
	if interval <= 0 {
		logx.Debugf("fallback polling is disabled")
		return
	}
	logx.Debugf("fallback polling every %s", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.pollOnce(ctx)
		}
	}
}

func (r *Runner) pollOnce(ctx context.Context) {
	ref, err := r.src.Resolve(ctx)
	if err != nil {
		if ctx.Err() == nil {
			logx.Warnf("poll failed: %v", err)
		}
		return
	}
	// current names what was last applied successfully, so a revision whose
	// build failed still looks new here and gets another attempt.
	current, ok := r.store.Pointer(source.PointerCurrent)
	if ok && current == ref {
		return
	}
	logx.Infof("poll found a new revision %s", short(ref))
	r.Trigger(control.Trigger{Kind: control.KindPoll, Ref: ref})
}
