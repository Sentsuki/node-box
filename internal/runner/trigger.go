package runner

// Kind identifies what asked for an update.
type Kind string

const (
	// KindStartup is the run performed as soon as the process starts.
	KindStartup Kind = "startup"
	// KindSchedule is the periodic re-fetch of subscriptions.
	KindSchedule Kind = "schedule"
	// KindPoll is the fallback poll noticing that the source ref changed.
	KindPoll Kind = "poll"
	// KindWebhook is a notification from the config repository.
	KindWebhook Kind = "webhook"
	// KindSignal is SIGHUP.
	KindSignal Kind = "signal"
	// KindManual is a one-shot CLI invocation.
	KindManual Kind = "manual"
)

// Trigger is one request to update.
type Trigger struct {
	Kind Kind
	// Ref is the snapshot to use. Empty means whatever the source currently
	// points at.
	Ref string
	// Force rewrites output files even when their content is unchanged.
	Force bool
}

// coalesce merges any triggers already queued into t.
//
// Bursts are common: a push fires the webhook while the fallback poll notices
// the same commit, and several pushes in a row each fire. Collapsing them into
// one run avoids doing identical work repeatedly, and the most recent ref is
// the one worth acting on.
func coalesce(t Trigger, queue <-chan Trigger) Trigger {
	for {
		select {
		case next := <-queue:
			t.Force = t.Force || next.Force
			if next.Ref != "" {
				t.Ref = next.Ref
			}
			// A specific request outranks a timer that happened to fire.
			if t.Kind == KindSchedule || t.Kind == KindPoll || t.Kind == KindStartup {
				t.Kind = next.Kind
			}
		default:
			return t
		}
	}
}
