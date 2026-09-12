package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"node-box/internal/control"
	"node-box/internal/logx"
	"node-box/internal/runner"
	"node-box/internal/source"
	"node-box/internal/webhook"
)

// cmdRun starts the daemon: the serial update loop, the schedule, the fallback
// poll and, when configured, the webhook server.
func cmdRun(ctx context.Context, env *env, args []string) error {
	fs := newFlagSet("run")
	env.bind(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	boot, err := env.loadBootstrap()
	if err != nil {
		return err
	}
	r, err := runner.New(boot, runner.Writable)
	if err != nil {
		return err
	}
	defer r.Close()

	// SIGHUP asks for an immediate update without restarting anything.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-hup:
				logx.Infof("SIGHUP received, updating")
				r.Trigger(control.Trigger{Kind: control.KindSignal})
			}
		}
	}()

	var wg sync.WaitGroup
	var serverErr error

	if boot.Server != nil && boot.Server.Enabled {
		secret := os.Getenv(boot.Server.WebhookSecretEnv)
		if secret == "" {
			return fmt.Errorf("environment variable %s is empty; it must hold the webhook secret",
				boot.Server.WebhookSecretEnv)
		}
		srv := webhook.New(boot.Server.Listen, []byte(secret), r)
		wg.Add(1)
		go func() {
			defer wg.Done()
			serverErr = srv.Run(ctx)
		}()
	} else {
		logx.Infof("webhook server disabled")
	}

	logx.Infof("%s %s started", appName, version)
	err = r.Run(ctx)
	wg.Wait()

	if err != nil {
		return err
	}
	return serverErr
}

// cmdUpdate performs exactly one update.
func cmdUpdate(ctx context.Context, env *env, args []string) error {
	fs := newFlagSet("update")
	env.bind(fs)
	ref := fs.String("ref", "", "snapshot to apply (default: whatever the source points at)")
	force := fs.Bool("force", false, "rewrite output files even when unchanged")
	if err := fs.Parse(args); err != nil {
		return err
	}

	r, err := newRunner(env, runner.Writable)
	if err != nil {
		return err
	}
	defer r.Close()

	return r.Execute(ctx, control.Trigger{Kind: control.KindManual, Ref: *ref, Force: *force})
}

// cmdPull refreshes the snapshot without generating anything.
func cmdPull(ctx context.Context, env *env, args []string) error {
	fs := newFlagSet("pull")
	env.bind(fs)
	ref := fs.String("ref", "", "snapshot to fetch (default: latest)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	r, err := newRunner(env, runner.ReadOnly)
	if err != nil {
		return err
	}
	defer r.Close()

	// No pointer moves here. The pointers describe what has been applied, and
	// pulling applies nothing. Having the snapshot on disk is the whole payload:
	// the next update finds it there instead of downloading it again.
	snap, err := source.Acquire(ctx, r.Source(), r.Store(), *ref)
	if err != nil {
		return err
	}
	fmt.Printf("snapshot %s ready at %s\n", snap.Ref, snap.Dir)
	return nil
}

// cmdBuild assembles the configuration without writing it. It never writes;
// use update to apply changes.
func cmdBuild(ctx context.Context, env *env, args []string) error {
	fs := newFlagSet("build")
	env.bind(fs)
	ref := fs.String("ref", "", "snapshot to assemble (default: whatever the source points at)")
	showDiff := fs.Bool("diff", false, "show what would change in each output file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	r, err := newRunner(env, runner.ReadOnly)
	if err != nil {
		return err
	}
	defer r.Close()

	plan, err := r.BuildPlan(ctx, *ref)
	if err != nil {
		return err
	}

	fmt.Printf("snapshot %s\n", plan.Snapshot.Ref)
	for _, f := range plan.Files {
		existing, err := os.ReadFile(f.Path)
		switch {
		case err != nil:
			fmt.Printf("\n+ %s (%s, new, %d bytes)\n", f.Path, f.Name, len(f.Content))
		case string(existing) == string(f.Content):
			fmt.Printf("\n= %s (%s, unchanged)\n", f.Path, f.Name)
			continue
		default:
			fmt.Printf("\n~ %s (%s, %d -> %d bytes)\n", f.Path, f.Name, len(existing), len(f.Content))
		}
		if *showDiff {
			printDiff(string(existing), string(f.Content))
		}
	}
	return nil
}

// cmdValidate checks that a snapshot assembles into valid configuration.
func cmdValidate(ctx context.Context, env *env, args []string) error {
	fs := newFlagSet("validate")
	env.bind(fs)
	ref := fs.String("ref", "", "snapshot to validate (default: whatever the source points at)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	r, err := newRunner(env, runner.ReadOnly)
	if err != nil {
		return err
	}
	defer r.Close()

	plan, err := r.Validate(ctx, *ref)
	if err != nil {
		return err
	}
	fmt.Printf("snapshot %s is valid; %d output file(s) would be generated:\n", plan.Snapshot.Ref, len(plan.Files))
	for _, f := range plan.Files {
		fmt.Printf("  %s (%s, %d bytes)\n", f.Path, f.Name, len(f.Content))
	}
	return nil
}

// cmdRollback regenerates output from the previously applied snapshot.
func cmdRollback(ctx context.Context, env *env, args []string) error {
	fs := newFlagSet("rollback")
	env.bind(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	r, err := newRunner(env, runner.Writable)
	if err != nil {
		return err
	}
	defer r.Close()

	return r.Rollback(ctx)
}

// cmdStatus prints what node-box has done so far.
func cmdStatus(_ context.Context, env *env, args []string) error {
	fs := newFlagSet("status")
	env.bind(fs)
	asJSON := fs.Bool("json", false, "print the status as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// No runner: status answers from the state file, the snapshot pointers and
	// the update lock, so it needs neither the repository token nor the lock.
	boot, err := env.loadBootstrap()
	if err != nil {
		return err
	}
	st := runner.ReadStatus(boot)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(st)
	}

	fmt.Printf("source:      %s\n", st.Source)
	fmt.Printf("applied:     %s\n", orNone(st.AppliedRef))
	fmt.Printf("previous:    %s\n", orNone(st.Previous))
	if st.Current != "" && st.Current != st.AppliedRef {
		// The two normally agree by construction, so a difference means the state
		// file was lost or hand-edited and is worth showing.
		fmt.Printf("pointer:     %s\n", st.Current)
	}
	fmt.Printf("daemon:      %s\n", daemonState(st))
	if st.LastTrigger != "" {
		fmt.Printf("last run:    %s", st.LastTrigger)
		if !st.StartedAt.IsZero() {
			fmt.Printf(" at %s", st.StartedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Println()
	}
	if !st.UpdatedAt.IsZero() {
		fmt.Printf("updated:     %s\n", st.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
	if st.LastError != "" {
		fmt.Printf("last error:  %s (%s)\n", st.LastError, st.LastErrorAt.Format("2006-01-02 15:04:05"))
	}
	if len(st.Outputs) > 0 {
		fmt.Println("outputs:")
		for path := range st.Outputs {
			fmt.Printf("  %s\n", path)
		}
	}
	return nil
}

// daemonState renders the three things the lock and the state file can say
// together.
func daemonState(st control.Status) string {
	switch {
	case st.Updating:
		return "running, update in progress"
	case st.Interrupted:
		return "not running; the last update was interrupted before it finished"
	case st.DaemonActive:
		return "running, idle"
	default:
		return "not running"
	}
}

// newRunner loads the bootstrap configuration and wires up a runner.
//
// The mode is not a detail: a Writable runner takes the root's lock, so a
// command that only reports must ask for ReadOnly or it would refuse to run
// while the daemon is up.
func newRunner(env *env, mode runner.Mode) (*runner.Runner, error) {
	boot, err := env.loadBootstrap()
	if err != nil {
		return nil, err
	}
	return runner.New(boot, mode)
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
