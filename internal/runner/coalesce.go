package runner

import "node-box/internal/control"

// coalesce merges any triggers already queued into t.
//
// Bursts are common: a push fires the webhook while the fallback poll notices
// the same commit, and several pushes in a row each fire. Collapsing them into
// one run avoids doing identical work repeatedly, and the most recent ref is
// the one worth acting on.
func coalesce(t control.Trigger, queue <-chan control.Trigger) control.Trigger {
	for {
		select {
		case next := <-queue:
			t.Force = t.Force || next.Force
			if next.Ref != "" {
				t.Ref = next.Ref
			}
			// A specific request outranks a timer that happened to fire.
			if t.Kind == control.KindSchedule || t.Kind == control.KindPoll || t.Kind == control.KindStartup {
				t.Kind = next.Kind
			}
		default:
			return t
		}
	}
}
