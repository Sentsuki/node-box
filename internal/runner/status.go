package runner

import (
	"context"
	"fmt"
	"time"

	"node-box/internal/logx"
	"node-box/internal/output"
	"node-box/internal/source"
)

// Status is a snapshot of what node-box has done so far. It is assembled from
// the state file and the snapshot pointers rather than kept in memory, so a
// one-shot CLI invocation reports the same thing the running daemon would.
type Status struct {
	Source      string            `json:"source"`
	Current     string            `json:"current,omitempty"`
	LastGood    string            `json:"last_good,omitempty"`
	AppliedRef  string            `json:"applied_ref,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitzero"`
	LastError   string            `json:"last_error,omitempty"`
	LastErrorAt time.Time         `json:"last_error_at,omitzero"`
	Running     bool              `json:"running"`
	LastTrigger string            `json:"last_trigger,omitempty"`
	Outputs     map[string]string `json:"outputs,omitempty"`
}

// Status reports the current state.
func (r *Runner) Status() Status {
	r.mu.Lock()
	running, lastKind := r.running, r.lastKind
	r.mu.Unlock()

	s := Status{
		Source:      r.src.Describe(),
		Running:     running,
		LastTrigger: string(lastKind),
	}
	if ref, ok := r.store.Pointer(source.PointerCurrent); ok {
		s.Current = ref
	}
	if ref, ok := r.store.Pointer(source.PointerLastGood); ok {
		s.LastGood = ref
	}
	if state, err := output.LoadState(r.boot.StateFile()); err == nil {
		s.AppliedRef = state.Ref
		s.UpdatedAt = state.UpdatedAt
		s.LastError = state.LastError
		s.LastErrorAt = state.LastErrorAt
		s.Outputs = state.Outputs
	}
	return s
}

// Rollback regenerates the outputs from the last snapshot that produced them.
//
// It is the escape hatch for a configuration that passed every check but turned
// out to be wrong in practice, which no amount of validation can catch.
func (r *Runner) Rollback(ctx context.Context) error {
	ref, ok := r.store.Pointer(source.PointerLastGood)
	if !ok {
		return fmt.Errorf("no last-good snapshot to roll back to")
	}
	current, _ := r.store.Pointer(source.PointerCurrent)
	if current == ref {
		logx.Infof("already at the last-good snapshot %s; regenerating outputs", short(ref))
	} else {
		logx.Infof("rolling back from %s to %s", short(current), short(ref))
	}

	if err := r.store.SetPointer(source.PointerCurrent, ref); err != nil {
		return err
	}
	// Force, because the output hashes on record already match this snapshot
	// and would otherwise skip every file.
	return r.Execute(ctx, Trigger{Kind: KindManual, Ref: ref, Force: true})
}

// Validate checks that a snapshot can be assembled into valid configurations,
// without writing anything.
func (r *Runner) Validate(ctx context.Context, ref string) (*Plan, error) {
	return r.BuildPlan(ctx, ref)
}
