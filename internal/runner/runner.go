// Package runner orchestrates one update: acquire a snapshot, fetch
// subscriptions, assemble, validate, write.
//
// Every trigger goes through a single serial loop. That is what keeps
// concurrent updates, duplicate webhooks and overlapping timers from needing
// any locking of their own.
package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"node-box/internal/build"
	"node-box/internal/fetch"
	"node-box/internal/logx"
	"node-box/internal/model"
	"node-box/internal/output"
	"node-box/internal/source"
	"node-box/internal/subscription"
)

// queueDepth is small on purpose: the coalescing step means a deep queue would
// only hold triggers that are about to be merged anyway.
const queueDepth = 8

// Runner owns the update pipeline.
type Runner struct {
	boot   *model.Bootstrap
	src    source.Source
	store  *source.Store
	client *fetch.Client

	triggers chan Trigger

	mu       sync.Mutex
	schedule *model.Schedule
	running  bool
	lastKind Kind
}

// New wires up a runner from the bootstrap configuration.
func New(boot *model.Bootstrap) (*Runner, error) {
	client, err := fetch.New(fetch.Options{
		Proxy:     boot.Proxy,
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

	store, err := source.NewStore(boot.SnapshotsDir())
	if err != nil {
		return nil, err
	}
	store.CleanScratch()

	return &Runner{
		boot:     boot,
		src:      src,
		store:    store,
		client:   client,
		triggers: make(chan Trigger, queueDepth),
	}, nil
}

// Source returns the configured source, for commands that inspect it.
func (r *Runner) Source() source.Source { return r.src }

// Store returns the snapshot store.
func (r *Runner) Store() *source.Store { return r.store }

// Trigger queues an update. It never blocks: if the queue is full an update is
// already pending, and that pending run will pick up the same work.
func (r *Runner) Trigger(t Trigger) bool {
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
	r.Trigger(Trigger{Kind: KindStartup})

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
func (r *Runner) Execute(ctx context.Context, t Trigger) error {
	r.setRunning(true, t.Kind)
	defer r.setRunning(false, t.Kind)

	started := time.Now()
	logx.Infof("update started (%s)", t.Kind)

	state, err := output.LoadState(r.boot.StateFile())
	if err != nil {
		return err
	}

	err = r.execute(ctx, t, state)
	if err != nil {
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
}

// BuildPlan runs the pipeline up to but not including writing.
//
// Exported so `build --dry-run` can show exactly what a real run would produce
// without touching a single file.
func (r *Runner) BuildPlan(ctx context.Context, ref string) (*Plan, error) {
	// 1. Snapshot. A source that cannot be reached falls back to the last one
	//    fetched rather than aborting: the outputs can still be regenerated
	//    from it, which is the whole point of keeping snapshots.
	snap, err := source.Acquire(ctx, r.src, r.store, ref)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
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
	} else if err := r.store.SetPointer(source.PointerCurrent, snap.Ref); err != nil {
		logx.Warnf("could not update the current pointer: %v", err)
	}

	r.setSchedule(snap.Config.UpdateSchedule)
	logx.Debugf("using snapshot %s", short(snap.Ref))

	// 2. Destinations. Checked before any fetching so a bad path fails fast.
	outs, err := snap.Config.ResolveOutputs(r.boot)
	if err != nil {
		return nil, err
	}
	if err := output.EnsureDirs(outs, snap.Config.OutputDir(r.boot)); err != nil {
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

	return &Plan{Snapshot: snap, Files: files}, nil
}

func (r *Runner) execute(ctx context.Context, t Trigger, state *output.State) error {
	plan, err := r.BuildPlan(ctx, t.Ref)
	if err != nil {
		return err
	}
	snap := plan.Snapshot

	// 6. Write.
	res, hashes, err := output.Write(plan.Files, t.Force)
	if err != nil {
		return err
	}
	logx.Infof("%d file(s) written, %d unchanged", len(res.Written), len(res.Skipped))
	for _, p := range res.Written {
		logx.Infof("  updated %s", p)
	}

	// 7. Record success.
	//
	// "previous" moves only when the applied ref actually changes, and it takes
	// the ref being replaced. Repeated runs of the same snapshot therefore do
	// not erase the version a rollback would return to.
	if applied := state.Ref; applied != "" && applied != snap.Ref {
		if err := r.store.SetPointer(source.PointerPrevious, applied); err != nil {
			logx.Debugf("could not record %s as the previous snapshot: %v", short(applied), err)
		}
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

func (r *Runner) setRunning(running bool, kind Kind) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = running
	r.lastKind = kind
}

func short(ref string) string {
	if len(ref) <= 8 {
		return ref
	}
	return ref[:8]
}
