package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"node-box/internal/fetch"
	"node-box/internal/logx"
	"node-box/internal/model"
)

// ConfigFileName is the repository configuration, at the repository root.
const ConfigFileName = "config.json"

// MaxModuleBytes caps a single module file.
const MaxModuleBytes = 8 << 20 // 8 MiB

// Snapshot is one immutable version of the configuration repository.
type Snapshot struct {
	// Ref identifies the snapshot: a commit sha for GitHub, a content hash
	// for a local directory.
	Ref string
	// Dir is where the snapshot lives on disk.
	Dir string
	// Config is the parsed and validated config.json.
	Config *model.Config
	// FetchedAt is when the snapshot was materialised.
	FetchedAt time.Time
}

// Source produces configuration snapshots.
type Source interface {
	// Resolve returns the ref the source currently points at.
	Resolve(ctx context.Context) (string, error)
	// Materialize writes the content of ref into destDir.
	Materialize(ctx context.Context, ref, destDir string) error
	// Describe names the source for logs.
	Describe() string
}

// Open loads a snapshot that is already on disk.
func Open(dir, ref string) (*Snapshot, error) {
	configPath := filepath.Join(dir, ConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", configPath, err)
	}
	cfg, err := model.LoadConfig(data)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(dir)
	var fetchedAt time.Time
	if err == nil {
		fetchedAt = info.ModTime()
	}

	return &Snapshot{Ref: ref, Dir: dir, Config: cfg, FetchedAt: fetchedAt}, nil
}

// Acquire returns the snapshot for ref, fetching it if it is not on disk.
// An empty ref means whatever the source currently points at.
func Acquire(ctx context.Context, src Source, store *Store, ref string) (*Snapshot, error) {
	if ref == "" {
		resolved, err := src.Resolve(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", src.Describe(), err)
		}
		ref = resolved
	}

	if store.Has(ref) {
		logx.Debugf("snapshot %s already present", ref)
		return Open(store.Dir(ref), ref)
	}

	tmp, err := store.Begin(ref)
	if err != nil {
		return nil, err
	}
	if err := src.Materialize(ctx, ref, tmp); err != nil {
		store.Abort(tmp)
		return nil, fmt.Errorf("fetch %s at %s: %w", src.Describe(), ref, err)
	}

	// Parse before committing so a snapshot that cannot even be read never
	// becomes a candidate for current.
	if _, err := Open(tmp, ref); err != nil {
		store.Abort(tmp)
		return nil, fmt.Errorf("snapshot %s is not usable: %w", ref, err)
	}
	if err := store.Commit(tmp, ref); err != nil {
		return nil, err
	}

	logx.Infof("fetched snapshot %s from %s", ref, src.Describe())
	return Open(store.Dir(ref), ref)
}

// LoadModules resolves every module the configuration's outputs actually use.
//
// Modules that no output references are skipped: an unused module with a
// broken URL should not be able to fail the whole run.
func (s *Snapshot) LoadModules(ctx context.Context, client *fetch.Client) (map[string]json.RawMessage, error) {
	used := make(map[string]bool)
	for _, cf := range s.Config.Configs {
		for _, name := range cf.Modules {
			used[name] = true
		}
	}

	out := make(map[string]json.RawMessage, len(used))
	for _, m := range s.Config.Modules {
		if !used[m.Name] {
			logx.Debugf("module %q is not referenced by any output, skipping", m.Name)
			continue
		}
		data, err := s.loadModule(ctx, client, m)
		if err != nil {
			return nil, fmt.Errorf("module %q (%s): %w", m.Name, m.Source(), err)
		}
		out[m.Name] = data
	}
	return out, nil
}

func (s *Snapshot) loadModule(ctx context.Context, client *fetch.Client, m model.Module) (json.RawMessage, error) {
	switch {
	case m.File != "":
		return readLimited(filepath.Join(s.Dir, filepath.FromSlash(m.File)))
	case m.Path != "":
		return readLimited(m.Path)
	case m.FromURL != "":
		resp, err := client.GetWithRetry(ctx, fetch.Request{
			URL:      m.FromURL,
			MaxBytes: MaxModuleBytes,
		}, fetch.DefaultRetry)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	default:
		return nil, fmt.Errorf("no source configured")
	}
}

// readLimited reads a file, refusing anything over MaxModuleBytes.
func readLimited(path string) (json.RawMessage, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxModuleBytes {
		return nil, fmt.Errorf("file is %d bytes, over the %d byte limit", info.Size(), MaxModuleBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}
