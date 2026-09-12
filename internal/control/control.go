// Package control holds the vocabulary for asking node-box to do something and
// for reporting what it has done.
//
// These types sit in their own dependency-free package so that the HTTP surface
// and the command line can speak to the update pipeline without importing it.
// An interface whose methods mention the implementation's own package is not
// actually decoupled from it, which is what the webhook server used to be.
package control

import "time"

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

// Status is a snapshot of what node-box has done so far.
//
// It is assembled from the state file, the snapshot pointers and the update
// lock rather than from memory, so a one-shot CLI invocation reports the same
// thing the running daemon would.
type Status struct {
	Source     string `json:"source"`
	Current    string `json:"current,omitempty"`
	Previous   string `json:"previous,omitempty"`
	AppliedRef string `json:"applied_ref,omitempty"`

	UpdatedAt   time.Time `json:"updated_at,omitzero"`
	LastError   string    `json:"last_error,omitempty"`
	LastErrorAt time.Time `json:"last_error_at,omitzero"`

	// DaemonActive reports whether some process currently holds the update
	// lock. It is read from the lock itself, so it is never stale: the kernel
	// releases the lock when the holder exits, crash included.
	DaemonActive bool `json:"daemon_active"`
	// Updating reports whether an update is in progress right now.
	Updating bool `json:"updating"`
	// Interrupted reports that a previous run started and never finished,
	// which is what a half-written state plus a released lock means.
	Interrupted bool `json:"interrupted,omitempty"`
	// LastTrigger is what asked for the most recent run.
	LastTrigger string `json:"last_trigger,omitempty"`
	// StartedAt is when that run began.
	StartedAt time.Time `json:"started_at,omitzero"`

	// Outputs maps each generated file to the hash of its content.
	Outputs map[string]string `json:"outputs,omitempty"`
}
