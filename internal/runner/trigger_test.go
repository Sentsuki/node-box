package runner

import (
	"testing"
	"time"

	"node-box/internal/control"
	"node-box/internal/model"
)

func TestCoalesce_MergesQueuedTriggers(t *testing.T) {
	q := make(chan control.Trigger, 8)
	q <- control.Trigger{Kind: control.KindWebhook, Ref: "aaa"}
	q <- control.Trigger{Kind: control.KindWebhook, Ref: "bbb"}

	// A timer fired first, but two pushes arrived behind it. The newest ref is
	// the one worth acting on, and the run should be attributed to the webhook.
	got := coalesce(control.Trigger{Kind: control.KindSchedule}, q)

	if got.Ref != "bbb" {
		t.Errorf("ref = %q, want the most recent (bbb)", got.Ref)
	}
	if got.Kind != control.KindWebhook {
		t.Errorf("kind = %q, want a specific request to outrank the timer", got.Kind)
	}
}

func TestCoalesce_KeepsForce(t *testing.T) {
	q := make(chan control.Trigger, 8)
	q <- control.Trigger{Kind: control.KindSchedule}

	got := coalesce(control.Trigger{Kind: control.KindManual, Force: true}, q)
	if !got.Force {
		t.Error("force must survive coalescing, or an explicit rewrite is silently dropped")
	}
}

func TestCoalesce_ManualKindIsNotDowngraded(t *testing.T) {
	q := make(chan control.Trigger, 8)
	q <- control.Trigger{Kind: control.KindSchedule}

	got := coalesce(control.Trigger{Kind: control.KindManual}, q)
	if got.Kind != control.KindManual {
		t.Errorf("kind = %q, want a timer not to override an explicit request", got.Kind)
	}
}

func TestCoalesce_EmptyQueueIsUnchanged(t *testing.T) {
	q := make(chan control.Trigger, 8)
	in := control.Trigger{Kind: control.KindStartup, Ref: "abc"}
	if got := coalesce(in, q); got != in {
		t.Errorf("coalesce changed a lone trigger: %+v", got)
	}
}

func TestNextScheduleDelay(t *testing.T) {
	now := time.Date(2026, 9, 11, 14, 37, 12, 0, time.UTC)

	t.Run("interval", func(t *testing.T) {
		s := &model.Schedule{Type: model.ScheduleInterval, Every: model.Duration(6 * time.Hour)}
		if got := nextScheduleDelay(s, now); got != 6*time.Hour {
			t.Errorf("delay = %s, want 6h", got)
		}
	})

	t.Run("hourly lands on the next hour", func(t *testing.T) {
		got := nextScheduleDelay(&model.Schedule{Type: model.ScheduleHourly}, now)
		want := 22*time.Minute + 48*time.Second
		if got != want {
			t.Errorf("delay = %s, want %s", got, want)
		}
	})

	t.Run("unknown schedule falls back", func(t *testing.T) {
		// Before the first snapshot is read there is no configured schedule,
		// and the loop must still make progress rather than spin.
		if got := nextScheduleDelay(nil, now); got != bootstrapScheduleDelay {
			t.Errorf("delay = %s, want %s", got, bootstrapScheduleDelay)
		}
	})
}
