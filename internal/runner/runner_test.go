package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"node-box/internal/model"
	"node-box/internal/output"
	"node-box/internal/source"
)

// env is a complete on-disk setup: a config repository read through the local
// source, and a state root for snapshots and outputs.
type env struct {
	t    *testing.T
	repo string
	root string
	boot *model.Bootstrap
}

const repoConfig = `{
  "nodes": {
    "subscriptions": [
      { "name": "own", "path": "nodes/own.json", "type": "singbox", "enable": true },
      { "name": "RL",  "path": "nodes/relay.json", "type": "singbox", "enable": true }
    ],
    "exclude_keywords": ["套餐到期"],
    "relays": [
      {
        "name": "via-jp",
        "via":      [{ "from": ["RL"], "include": ["JP"] }],
        "upstream": [{ "from": ["own"], "include": ["tokyo"] }]
      }
    ]
  },
  "modules": [
    { "name": "log", "file": "modules/log.json" },
    {
      "name": "out",
      "file": "modules/out.json",
      "selectors": [
        { "tag": "Proxy", "from": ["own"], "relays": ["via-jp"] }
      ]
    }
  ],
  "configs": [
    { "name": "main", "path": "main.json", "modules": ["log", "out"] }
  ],
  "update_schedule": { "type": "hourly" }
}`

var repoFiles = map[string]string{
	"config.json":      repoConfig,
	"modules/log.json": `{"log":{"level":"info"}}`,
	"modules/out.json": `{"outbounds":[
      {"type":"direct","tag":"direct"},
      {"type":"selector","tag":"Proxy","outbounds":["direct"]}
    ]}`,
	"nodes/own.json": `{"outbounds":[
      {"type":"vmess","tag":"tokyo","server":"1.1.1.1","server_port":443,"uuid":"u"},
      {"type":"vmess","tag":"osaka","server":"1.1.1.2","server_port":443,"uuid":"u"},
      {"type":"vmess","tag":"套餐到期 2026-01-01","server":"x","server_port":1,"uuid":"u"}
    ]}`,
	"nodes/relay.json": `{"outbounds":[
      {"type":"vmess","tag":"JP","server":"jp.relay","server_port":443,"uuid":"u"}
    ]}`,
}

func newEnv(t *testing.T) *env {
	t.Helper()
	repo, root := t.TempDir(), t.TempDir()

	for name, content := range repoFiles {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return &env{
		t:    t,
		repo: repo,
		root: root,
		boot: &model.Bootstrap{
			Root:     root,
			LogLevel: "error",
			Source:   &model.SourceConfig{Type: model.SourceLocal, Dir: repo},
		},
	}
}

// runner builds a runner that may generate output. It holds the root's lock,
// so a test wanting a second one must let this one go first.
func (e *env) runner() *Runner {
	e.t.Helper()
	r, err := New(e.boot, Writable)
	if err != nil {
		e.t.Fatalf("New: %v", err)
	}
	e.t.Cleanup(func() { r.Close() })
	return r
}

// reader builds a runner that only inspects, the way status and build do.
func (e *env) reader() *Runner {
	e.t.Helper()
	r, err := New(e.boot, ReadOnly)
	if err != nil {
		e.t.Fatalf("New(ReadOnly): %v", err)
	}
	e.t.Cleanup(func() { r.Close() })
	return r
}

// currentRef returns what the store believes produced the files on disk.
func (e *env) currentRef() string {
	e.t.Helper()
	ref, _ := e.snapshotStore().Pointer(source.PointerCurrent)
	return ref
}

func (e *env) snapshotStore() *source.Store {
	e.t.Helper()
	s, err := source.NewStore(e.boot.SnapshotsDir())
	if err != nil {
		e.t.Fatalf("NewStore: %v", err)
	}
	return s
}

// update runs one full update.
func (e *env) update(r *Runner) error {
	return r.Execute(context.Background(), Trigger{Kind: KindManual})
}

func (e *env) outputPath() string {
	return filepath.Join(e.root, "out", "main.json")
}

func (e *env) readOutput() string {
	e.t.Helper()
	data, err := os.ReadFile(e.outputPath())
	if err != nil {
		e.t.Fatalf("read output: %v", err)
	}
	return string(data)
}

// writeRepo replaces a file in the config repository.
func (e *env) writeRepo(name, content string) {
	e.t.Helper()
	path := filepath.Join(e.repo, filepath.FromSlash(name))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) state() *output.State {
	e.t.Helper()
	s, err := output.LoadState(e.boot.StateFile())
	if err != nil {
		e.t.Fatalf("LoadState: %v", err)
	}
	return s
}

func TestRunner_FullPipeline(t *testing.T) {
	e := newEnv(t)
	r := e.runner()

	if err := e.update(r); err != nil {
		t.Fatalf("update: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(e.readOutput()), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// The selector drives everything: literal member, then the relay, then the
	// regular nodes it selected.
	arr := doc["outbounds"].([]any)
	var members []string
	for _, raw := range arr {
		obj := raw.(map[string]any)
		if obj["tag"] == "Proxy" {
			for _, m := range obj["outbounds"].([]any) {
				members = append(members, m.(string))
			}
		}
	}
	want := []string{"direct", "[RL] JP [own] tokyo", "[own] tokyo", "[own] osaka"}
	if strings.Join(members, "|") != strings.Join(want, "|") {
		t.Errorf("Proxy members:\n got %q\nwant %q", members, want)
	}

	// The junk entry the airport shipped never enters the pool.
	if strings.Contains(e.readOutput(), "套餐到期") {
		t.Error("a node matching exclude_keywords reached the output")
	}
	// A relay template is not a node in its own right.
	if strings.Contains(e.readOutput(), `"tag": "[RL] JP"`) {
		t.Error("the relay template was inserted as a node")
	}

	st := e.state()
	if st.Ref == "" {
		t.Error("state did not record the applied ref")
	}
	if st.LastError != "" {
		t.Errorf("state recorded an error: %s", st.LastError)
	}
	// Only one version has ever been applied, so there is nothing to roll
	// back to yet.
	if ref, ok := r.Store().Pointer(source.PointerPrevious); ok {
		t.Errorf("previous = %q after the first run, want it unset", ref)
	}
}

func TestRunner_SecondRunIsIdempotent(t *testing.T) {
	e := newEnv(t)
	r := e.runner()

	if err := e.update(r); err != nil {
		t.Fatal(err)
	}
	first := e.readOutput()

	if err := e.update(r); err != nil {
		t.Fatal(err)
	}
	if e.readOutput() != first {
		t.Error("a second update with unchanged inputs produced different output")
	}
}

func TestRunner_BadConfigLeavesOutputUntouched(t *testing.T) {
	e := newEnv(t)
	r := e.runner()

	if err := e.update(r); err != nil {
		t.Fatal(err)
	}
	good := e.readOutput()
	goodRef := e.state().Ref

	// Break the repo: two modules now define the same top-level key.
	e.writeRepo("modules/log.json", `{"outbounds":[{"type":"direct","tag":"clash"}]}`)

	if err := e.update(r); err == nil {
		t.Fatal("want an error for a repo with conflicting modules")
	}

	// The documented rule: a failure writes nothing.
	if e.readOutput() != good {
		t.Error("a failed update modified the existing output")
	}
	st := e.state()
	if st.LastError == "" {
		t.Error("the failure was not recorded in state")
	}
	if st.Ref != goodRef {
		t.Errorf("applied ref moved to %q on failure, want it to stay at %q", st.Ref, goodRef)
	}
	// A failed run must not touch the rollback target either.
	if ref, ok := r.Store().Pointer(source.PointerPrevious); ok {
		t.Errorf("previous = %q, want it still unset after a failed run", ref)
	}
}

func TestRunner_EmptySelectorIsRejected(t *testing.T) {
	e := newEnv(t)
	r := e.runner()

	// A rule that matches nothing leaves the group with only "direct" here, so
	// narrow the module to a group with no literal members.
	e.writeRepo("modules/out.json", `{"outbounds":[
      {"type":"direct","tag":"direct"},
      {"type":"selector","tag":"Proxy"}
    ]}`)
	e.writeRepo("config.json", strings.Replace(repoConfig,
		`{ "tag": "Proxy", "from": ["own"], "relays": ["via-jp"] }`,
		`{ "tag": "Proxy", "from": ["own"], "include": ["no-such-node"] }`, 1))

	err := e.update(r)
	if err == nil || !strings.Contains(err.Error(), "no members") {
		t.Fatalf("want an empty-group error, got %v", err)
	}
	if _, statErr := os.Stat(e.outputPath()); statErr == nil {
		t.Error("a rejected build should not have produced a file")
	}
}

func TestRunner_AllSubscriptionsFailingWritesNothing(t *testing.T) {
	e := newEnv(t)
	r := e.runner()

	// Point every subscription at a file that does not exist. Regenerating from
	// zero nodes would empty the configuration, so the run must abort instead.
	e.writeRepo("config.json", strings.NewReplacer(
		`"path": "nodes/own.json"`, `"path": "nodes/missing.json"`,
		`"path": "nodes/relay.json"`, `"path": "nodes/gone.json"`,
	).Replace(repoConfig))

	err := e.update(r)
	if err == nil || !strings.Contains(err.Error(), "all") {
		t.Fatalf("want a total-failure error, got %v", err)
	}
	if _, statErr := os.Stat(e.outputPath()); statErr == nil {
		t.Error("nothing should have been written")
	}
}

func TestRunner_Rollback(t *testing.T) {
	e := newEnv(t)
	r := e.runner()

	if err := e.update(r); err != nil {
		t.Fatal(err)
	}
	good := e.readOutput()
	goodRef := e.state().Ref

	// A change that is valid but unwanted.
	e.writeRepo("modules/log.json", `{"log":{"level":"trace"}}`)
	if err := e.update(r); err != nil {
		t.Fatal(err)
	}
	if e.readOutput() == good {
		t.Fatal("the second update should have changed the output")
	}

	if err := r.Rollback(context.Background()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if e.readOutput() != good {
		t.Error("rollback did not restore the previous output")
	}
	if got := e.state().Ref; got != goodRef {
		t.Errorf("applied ref after rollback = %q, want %q", got, goodRef)
	}
}

func TestRunner_FallsBackToCurrentSnapshotWhenSourceIsGone(t *testing.T) {
	e := newEnv(t)
	r := e.runner()

	if err := e.update(r); err != nil {
		t.Fatal(err)
	}
	good := e.readOutput()

	// Simulate the source becoming unreachable. The snapshot on disk is still
	// a complete copy, so the outputs can be regenerated from it.
	if err := os.RemoveAll(e.repo); err != nil {
		t.Fatal(err)
	}

	if err := r.Execute(context.Background(), Trigger{Kind: KindManual, Force: true}); err != nil {
		t.Fatalf("update with an unreachable source: %v", err)
	}
	if e.readOutput() != good {
		t.Error("the fallback run produced different output")
	}
}

func TestRunner_RollbackNeedsAPreviousVersion(t *testing.T) {
	e := newEnv(t)
	r := e.runner()

	if err := e.update(r); err != nil {
		t.Fatal(err)
	}
	// One version applied: there is nothing earlier to return to, and saying so
	// is better than silently regenerating the same thing.
	err := r.Rollback(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no previous snapshot") {
		t.Fatalf("want a no-previous-version error, got %v", err)
	}
}

func TestRunner_RepeatedRunsKeepTheRollbackTarget(t *testing.T) {
	e := newEnv(t)
	r := e.runner()

	if err := e.update(r); err != nil {
		t.Fatal(err)
	}
	first := e.state().Ref

	e.writeRepo("modules/log.json", `{"log":{"level":"trace"}}`)
	if err := e.update(r); err != nil {
		t.Fatal(err)
	}

	// Running again without changing anything must not push the rollback
	// target forward, or a scheduled re-fetch would quietly destroy it.
	for range 3 {
		if err := e.update(r); err != nil {
			t.Fatal(err)
		}
	}
	if ref, _ := r.Store().Pointer(source.PointerPrevious); ref != first {
		t.Errorf("previous = %q after repeated runs, want %q", ref, first)
	}
}

// --- the update lock -------------------------------------------------------

func TestRunner_ReadOnlyRunnerRefusesToExecute(t *testing.T) {
	e := newEnv(t)
	r := e.reader()

	err := r.Execute(context.Background(), Trigger{Kind: KindManual})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("want a read-only refusal, got %v", err)
	}
	if _, statErr := os.Stat(e.outputPath()); statErr == nil {
		t.Error("a read-only runner produced output")
	}
}

func TestRunner_SecondWriterIsRefused(t *testing.T) {
	e := newEnv(t)
	e.runner() // holds the lock for the rest of the test

	_, err := New(e.boot, Writable)
	if err == nil {
		t.Fatal("want a lock conflict for a second writable runner")
	}
	if !strings.Contains(err.Error(), "another node-box process") {
		t.Errorf("the error should explain the conflict, got %v", err)
	}
}

func TestRunner_ReadersRunAlongsideAWriter(t *testing.T) {
	e := newEnv(t)
	w := e.runner()
	if err := e.update(w); err != nil {
		t.Fatal(err)
	}

	// This is the case that used to corrupt an in-flight fetch: a reporting
	// command starting while the daemon owns the root.
	r, err := New(e.boot, ReadOnly)
	if err != nil {
		t.Fatalf("a read-only runner must not need the lock: %v", err)
	}
	defer r.Close()

	if st := r.Status(); st.AppliedRef == "" {
		t.Error("the reader could not see what the writer applied")
	}
}

func TestRunner_WriterLockIsReusableAfterClose(t *testing.T) {
	e := newEnv(t)

	first, err := New(e.boot, Writable)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := New(e.boot, Writable)
	if err != nil {
		t.Fatalf("the lock was not released: %v", err)
	}
	defer second.Close()
}

// --- an explicit ref is never substituted ---------------------------------

func TestRunner_UnavailableExplicitRefIsAnError(t *testing.T) {
	e := newEnv(t)
	r := e.runner()

	if err := e.update(r); err != nil {
		t.Fatal(err)
	}
	good := e.readOutput()
	appliedRef := e.state().Ref

	// The same unreachable source that an empty ref is allowed to fall back
	// from. Asking for a specific ref is a different question, and the honest
	// answer is that it cannot be served: falling back would report success for
	// a version that was never applied.
	if err := os.RemoveAll(e.repo); err != nil {
		t.Fatal(err)
	}

	err := r.Execute(context.Background(), Trigger{Kind: KindManual, Ref: "deadbeefdeadbeef"})
	if err == nil {
		t.Fatal("want an error for an unavailable explicit ref")
	}
	if !strings.Contains(err.Error(), "requested explicitly") {
		t.Errorf("the error should say the ref was explicit, got %v", err)
	}
	if e.readOutput() != good {
		t.Error("the failed run rewrote the output")
	}
	if got := e.state().Ref; got != appliedRef {
		t.Errorf("applied ref moved to %q, want it to stay at %q", got, appliedRef)
	}
}

func TestRunner_RollbackFailsLoudlyOnAnUnusablePrevious(t *testing.T) {
	e := newEnv(t)
	r := e.runner()

	if err := e.update(r); err != nil {
		t.Fatal(err)
	}
	e.writeRepo("modules/log.json", `{"log":{"level":"trace"}}`)
	if err := e.update(r); err != nil {
		t.Fatal(err)
	}
	latest := e.readOutput()

	previous, ok := r.Store().Pointer(source.PointerPrevious)
	if !ok {
		t.Fatal("previous should be set after two different snapshots")
	}
	// Break the snapshot rollback is supposed to return to.
	broken := filepath.Join(e.boot.SnapshotsDir(), previous, "config.json")
	if err := os.WriteFile(broken, []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Reporting success while leaving the unwanted version in place is the one
	// outcome rollback must never produce.
	err := r.Rollback(context.Background())
	if err == nil {
		t.Fatal("want an error when the rollback target cannot be read")
	}
	if e.readOutput() != latest {
		t.Error("output changed even though the rollback failed")
	}
}

// --- planning has no side effects ----------------------------------------

func TestRunner_BuildPlanWritesNothing(t *testing.T) {
	e := newEnv(t)
	r := e.reader()

	plan, err := r.BuildPlan(context.Background(), "")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Files) == 0 {
		t.Fatal("the plan should contain the file it would write")
	}

	if _, err := os.Stat(filepath.Join(e.root, "out")); err == nil {
		t.Error("planning created the output directory")
	}
	if _, err := os.Stat(e.boot.StateFile()); err == nil {
		t.Error("planning wrote the state file")
	}
	if ref := e.currentRef(); ref != "" {
		t.Errorf("planning moved the current pointer to %q", ref)
	}
}

func TestRunner_CurrentPointerFollowsWhatWasWritten(t *testing.T) {
	e := newEnv(t)
	r := e.runner()

	if err := e.update(r); err != nil {
		t.Fatal(err)
	}
	applied := e.state().Ref
	if got := e.currentRef(); got != applied {
		t.Fatalf("current = %q after a successful run, want the applied ref %q", got, applied)
	}

	// A revision that cannot be built must not claim the pointer, or the
	// fallback would restore it and the change poll would treat it as done.
	e.writeRepo("modules/log.json", `{"outbounds":[{"type":"direct","tag":"clash"}]}`)
	if err := e.update(r); err == nil {
		t.Fatal("want an error for a repo with conflicting modules")
	}
	if got := e.currentRef(); got != applied {
		t.Errorf("current = %q after a failed run, want it to stay at %q", got, applied)
	}
}
