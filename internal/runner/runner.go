// Package runner orchestrates one update: acquire a snapshot, fetch
// subscriptions, assemble, validate, write.
//
// Every trigger goes through a single serial loop, which is what keeps
// concurrent updates, duplicate webhooks and overlapping timers from needing
// any locking of their own inside one process. Across processes the same
// guarantee comes from the lock a Writable runner holds for its whole lifetime:
// a runner built ReadOnly cannot execute an update at all, so the serial loop
// really is the only thing that ever writes.
package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"node-box/internal/build"
	"node-box/internal/control"
	"node-box/internal/fetch"
	"node-box/internal/lockfile"
	"node-box/internal/logx"
	"node-box/internal/model"
	"node-box/internal/output"
	"node-box/internal/source"
	"node-box/internal/subscription"
)

// queueDepth is small on purpose: the coalescing step means a deep queue would
// only hold triggers that are about to be merged anyway.
const queueDepth = 8

// Mode says whether a runner is allowed to change anything.
type Mode int

const (
	// ReadOnly inspects what is already on disk. It takes no lock, so it is
	// always safe to run alongside a daemon, and Execute refuses to run.
	//
	// Acquiring a snapshot is still permitted: scratch directories have unique
	// names and committing one is idempotent, so fetching cannot disturb
	// another process.
	ReadOnly Mode = iota
	// Writable may generate output. It holds the root's lock for its whole
	// lifetime and must be closed.
	Writable
)

// Runner owns the update pipeline.
type Runner struct {
	boot   *model.Bootstrap
	src    source.Source
	store  *source.Store
	client *fetch.Client
	mode   Mode
	lock   *lockfile.Lock

	triggers chan control.Trigger

	mu       sync.Mutex
	schedule *model.Schedule
}

// New wires up a runner from the bootstrap configuration.
//
// A Writable runner takes the root's lock before touching anything and the
// caller must Close it. Pass ReadOnly for commands that only report.
func New(boot *model.Bootstrap, mode Mode) (*Runner, error) {
	client, err := fetch.New(fetch.Options{
		ProxyURL:  boot.Proxy.URL(),
		UserAgent: "node-box",
	})
	if err != nil {
		return nil, err
	}

	var src source.Source
	switch boot.Source.Type {
	case model.SourceGitHub:
		token := os.Getenv(boot.Source.TokenEnv)
		if token == "" {
			return nil, fmt.Errorf("environment variable %s is empty; it must hold the GitHub token", boot.Source.TokenEnv)
		}
		src, err = source.NewGitHub(client, boot.Source.Repo, boot.Source.Branch, token)
		if err != nil {
			return nil, err
		}
	case model.SourceLocal:
		src = source.NewLocal(boot.Source.Dir)
	default:
		return nil, fmt.Errorf("unknown source type %q", boot.Source.Type)
	}

	r := &Runner{
		boot:     boot,
		src:      src,
		client:   client,
		mode:     mode,
		triggers: make(chan control.Trigger, queueDepth),
	}

	if mode == Writable {
		lock, err := lockfile.Acquire(boot.LockFile())
		if err != nil {
			if errors.Is(err, lockfile.ErrLocked) {
				return nil, fmt.Errorf(
					"another node-box process is already updating %s; "+
						"send SIGHUP to the running daemon to make it update now, or stop it first", boot.Root)
			}
			return nil, err
		}
		r.lock = lock
	}

	store, err := source.NewStore(boot.SnapshotsDir())
	if err != nil {
		r.Close()
		return nil, err
	}
	r.store = store

	// Only safe behind the lock: a scratch directory that looks abandoned may
	// belong to a fetch another process is in the middle of.
	if mode == Writable {
		store.CleanScratch()
	}

	return r, nil
}

// Close releases the update lock. It is safe to call on a ReadOnly runner and
// safe to call twice.
func (r *Runner) Close() error { return r.lock.Release() }

// Source returns the configured source, for commands that inspect it.
func (r *Runner) Source() source.Source { return r.src }

// Store returns the snapshot store.
func (r *Runner) Store() *source.Store { return r.store }

// Trigger queues an update. It never blocks: if the queue is full an update is
// already pending, and that pending run will pick up the same work.
func (r *Runner) Trigger(t control.Trigger) bool {
	select {
	case r.triggers <- t:
		return true
	default:
		logx.Debugf("trigger %s dropped, an update is already queued", t.Kind)
		return false
	}
}

// Run consumes triggers until the context is cancelled. It also starts the
// schedule and fallback poll loops.
func (r *Runner) Run(ctx context.Context) error {
	r.Trigger(control.Trigger{Kind: control.KindStartup})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); r.scheduleLoop(ctx) }()
	go func() { defer wg.Done(); r.pollLoop(ctx) }()

	for {
		select {
		case <-ctx.Done():
			logx.Infof("shutting down")
			wg.Wait()
			return nil

		case t := <-r.triggers:
			t = coalesce(t, r.triggers)
			if err := r.Execute(ctx, t); err != nil {
				if errors.Is(err, context.Canceled) {
					continue
				}
				logx.Errorf("update failed (%s): %v", t.Kind, err)
			}
		}
	}
}

// Execute performs one complete update.
//
// Nothing is written until every step has succeeded, so any failure leaves the
// previously generated files exactly as they were.
//
// The run is bounded by update_timeout. Without one, a single stalled fetch
// holds the serial trigger loop forever and the daemon stops updating with no
// error to show for it — the HTTP client's per-request timeout does not bound a
// run that keeps making progress slowly across many subscriptions.
func (r *Runner) Execute(ctx context.Context, t control.Trigger) error {
	if r.mode != Writable {
		return fmt.Errorf("this runner is read-only and cannot generate output")
	}

	if limit := r.boot.UpdateTimeout.Duration(); limit > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, limit)
		defer cancel()
	}

	started := time.Now()
	logx.Infof("update started (%s)", t.Kind)

	state, err := output.LoadState(r.boot.StateFile())
	if err != nil {
		return err
	}

	// Persist "a run is under way" before doing any of it, so a status command
	// in another process can see it, and so a crash mid-run stays visible.
	state.RecordStart(string(t.Kind))
	if saveErr := state.Save(r.boot.StateFile()); saveErr != nil {
		logx.Warnf("could not record the start of this run: %v", saveErr)
	}

	err = r.execute(ctx, t, state)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("update exceeded update_timeout (%s): %w", r.boot.UpdateTimeout, err)
		}
		state.RecordFailure(err)
		if saveErr := state.Save(r.boot.StateFile()); saveErr != nil {
			logx.Warnf("could not record failure in state: %v", saveErr)
		}
		return err
	}

	logx.Infof("update finished in %s", time.Since(started).Round(time.Millisecond))
	return nil
}

// Plan is the result of assembling everything, before anything is written.
type Plan struct {
	Snapshot *source.Snapshot
	Files    []output.File
	// Outputs are the resolved destinations the files belong to, carried so the
	// writing step does not have to resolve them a second time.
	Outputs []output.Target
}

// BuildPlan runs the pipeline up to but not including writing.
//
// It writes nothing at all — no files, no directories, no snapshot pointers —
// so `build` and `validate` can show exactly what a real run would produce
// without disturbing a daemon sharing the same root. Acquiring a snapshot may
// still download one, which is safe from any number of processes.
func (r *Runner) BuildPlan(ctx context.Context, ref string) (*Plan, error) {
	// 1. Snapshot.
	snap, err := r.acquire(ctx, ref)
	if err != nil {
		return nil, err
	}

	r.setSchedule(snap.Config.UpdateSchedule)
	logx.Debugf("using snapshot %s", short(snap.Ref))

	// 2. Destinations. Inspected before any fetching so a bad path fails fast.
	//    Only inspected: the directories are created by the writing step, which
	//    is the first point at which creating them is warranted.
	outs, err := output.Resolve(snap.Config, r.boot)
	if err != nil {
		return nil, err
	}
	if err := output.CheckDirs(outs, output.BaseDir(snap.Config, r.boot)); err != nil {
		return nil, err
	}

	// 3. Modules.
	modules, err := snap.LoadModules(ctx, r.client)
	if err != nil {
		return nil, err
	}

	// 4. Subscriptions. Partial failures are tolerated; a total failure is not,
	//    because regenerating from zero nodes would empty the configurations.
	nodes, err := subscription.NewFetcher(r.client, snap.Dir).FetchAll(ctx, snap.Config)
	if err != nil {
		return nil, err
	}

	// 5. Assemble and validate, entirely in memory.
	files, err := build.Build(build.Input{
		Config:  snap.Config,
		Modules: modules,
		Nodes:   nodes,
		Outputs: outs,
	})
	if err != nil {
		return nil, err
	}

	return &Plan{Snapshot: snap, Files: files, Outputs: outs}, nil
}

// acquire returns the snapshot to build from.
//
// When no ref was asked for, a source that cannot be reached falls back to the
// snapshot that produced the current outputs rather than aborting: those
// outputs can still be regenerated from it, which is the whole point of keeping
// snapshots.
//
// An explicit ref never falls back. Quietly building something else would turn
// `update --ref` into a lie, and would reduce `rollback` — whose whole job is
// to apply a ref other than the current one — to a no-op that reports success.
func (r *Runner) acquire(ctx context.Context, ref string) (*source.Snapshot, error) {
	snap, err := source.Acquire(ctx, r.src, r.store, ref)
	if err == nil {
		return snap, nil
	}
	if ctx.Err() != nil {
		return nil, err
	}
	if ref != "" {
		return nil, fmt.Errorf("snapshot %s was requested explicitly and is unavailable: %w", short(ref), err)
	}

	fallback, ok := r.store.Pointer(source.PointerCurrent)
	if !ok {
		return nil, fmt.Errorf("no snapshot available: %w", err)
	}
	logx.Warnf("could not fetch configuration (%v); falling back to snapshot %s", err, short(fallback))

	snap, err = source.Open(r.store.Dir(fallback), fallback)
	if err != nil {
		return nil, fmt.Errorf("fallback snapshot %s is unusable: %w", short(fallback), err)
	}
	return snap, nil
}

func (r *Runner) execute(ctx context.Context, t control.Trigger, state *output.State) error {
	plan, err := r.BuildPlan(ctx, t.Ref)
	if err != nil {
		return err
	}
	snap := plan.Snapshot

	// 6. Prepare the destinations. This is the first step that changes anything
	//    on disk, and it happens only once the whole build has succeeded.
	if err := output.EnsureDirs(plan.Outputs, output.BaseDir(snap.Config, r.boot)); err != nil {
		return err
	}

	// 7. Write.
	res, hashes, err := output.Write(plan.Files, t.Force)
	if err != nil {
		return err
	}
	logx.Infof("%d file(s) written, %d unchanged", len(res.Written), len(res.Skipped))
	for _, p := range res.Written {
		logx.Infof("  updated %s", p)
	}

	// 8. Record success.
	//
	// "previous" moves only when the applied ref actually changes, and it takes
	// the ref being replaced. Repeated runs of the same snapshot therefore do
	// not erase the version a rollback would return to.
	if applied := state.Ref; applied != "" && applied != snap.Ref {
		if err := r.store.SetPointer(source.PointerPrevious, applied); err != nil {
			logx.Debugf("could not record %s as the previous snapshot: %v", short(applied), err)
		}
	}
	// "current" moves last, so it only ever names a snapshot that really did
	// produce the files on disk. A snapshot that failed to build leaves it
	// alone, which is what lets the fallback path trust it and the poll notice
	// the same ref again and retry.
	if err := r.store.SetPointer(source.PointerCurrent, snap.Ref); err != nil {
		logx.Warnf("could not update the current pointer: %v", err)
	}
	state.RecordSuccess(snap.Ref, hashes)
	if err := state.Save(r.boot.StateFile()); err != nil {
		logx.Warnf("could not save state: %v", err)
	}
	if err := r.store.GC(source.DefaultKeep); err != nil {
		logx.Warnf("snapshot cleanup failed: %v", err)
	}
	return nil
}

func (r *Runner) setSchedule(s *model.Schedule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedule = s
}

func (r *Runner) currentSchedule() *model.Schedule {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.schedule
}

func short(ref string) string {
	if len(ref) <= 8 {
		return ref
	}
	return ref[:8]
}
