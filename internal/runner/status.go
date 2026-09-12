package runner

import (
	"context"
	"fmt"

	"node-box/internal/control"
	"node-box/internal/lockfile"
	"node-box/internal/logx"
	"node-box/internal/model"
	"node-box/internal/output"
	"node-box/internal/source"
)

// ReadStatus reports what node-box has done, reading only the state file, the
// snapshot pointers and the update lock.
//
// It builds no HTTP client and no source, so it works without the repository
// token: asking a read-only question should not require a credential that only
// fetching needs. It is also why a one-shot CLI invocation and the daemon's own
// HTTP endpoint cannot disagree — both answer from the same files.
func ReadStatus(boot *model.Bootstrap) control.Status {
	s := control.Status{
		Source: boot.Source.Describe(),
		// Taken from the lock rather than from memory. The kernel releases it
		// when the holder exits, crash included, so unlike a flag written into a
		// file this cannot go stale.
		DaemonActive: lockfile.IsHeld(boot.LockFile()),
	}

	store := source.OpenStore(boot.SnapshotsDir())
	if ref, ok := store.Pointer(source.PointerCurrent); ok {
		s.Current = ref
	}
	if ref, ok := store.Pointer(source.PointerPrevious); ok {
		s.Previous = ref
	}

	state, err := output.LoadState(boot.StateFile())
	if err != nil {
		return s
	}
	s.AppliedRef = state.Ref
	s.UpdatedAt = state.UpdatedAt
	s.LastError = state.LastError
	s.LastErrorAt = state.LastErrorAt
	s.LastTrigger = state.LastTrigger
	s.StartedAt = state.StartedAt
	s.Outputs = state.Outputs

	// A run that recorded its start but never recorded an outcome is either
	// still going or died partway. Which one it is comes from the lock.
	if state.Updating {
		s.Updating = s.DaemonActive
		s.Interrupted = !s.DaemonActive
	}
	return s
}

// Status reports the current state.
func (r *Runner) Status() control.Status { return ReadStatus(r.boot) }

// Rollback regenerates the outputs from the snapshot applied before the
// current one.
//
// It is the escape hatch for a configuration that passed every check but turned
// out to be wrong in practice, which no amount of validation can catch.
func (r *Runner) Rollback(ctx context.Context) error {
	ref, ok := r.store.Pointer(source.PointerPrevious)
	if !ok {
		return fmt.Errorf("no previous snapshot to roll back to; only one version has ever been applied")
	}

	state, err := output.LoadState(r.boot.StateFile())
	if err != nil {
		return err
	}
	logx.Infof("rolling back from %s to %s", short(state.Ref), short(ref))

	// Force, because the outputs on disk already match the snapshot being
	// replaced and a content comparison would skip every file.
	return r.Execute(ctx, control.Trigger{Kind: control.KindManual, Ref: ref, Force: true})
}

// Validate checks that a snapshot can be assembled into valid configurations.
// It writes nothing.
func (r *Runner) Validate(ctx context.Context, ref string) (*Plan, error) {
	return r.BuildPlan(ctx, ref)
}
