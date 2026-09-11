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

func (e *env) runner() *Runner {
	e.t.Helper()
	r, err := New(e.boot)
	if err != nil {
		e.t.Fatalf("New: %v", err)
	}
	return r
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
	want := []string{"direct", "[RL] JP → [own] tokyo", "[own] tokyo", "[own] osaka"}
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
