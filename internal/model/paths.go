package model

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResolvedOutput pairs an output definition with its absolute destination.
type ResolvedOutput struct {
	Config ConfigFile
	Path   string
}

// OutputDir returns the absolute base directory for relative output paths.
// A relative output.dir is resolved against the bootstrap root; an empty one
// defaults to <root>/out.
func (c *Config) OutputDir(b *Bootstrap) string {
	if c.Output == nil || c.Output.Dir == "" {
		return b.DefaultOutputDir()
	}
	dir := c.Output.Dir
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(b.Root, dir)
	}
	return filepath.Clean(dir)
}

// ResolveOutputs turns every configs[] entry into an absolute destination path.
//
// Resolution rules:
//   - an absolute path is used as-is and ignores output.dir
//   - a relative path is resolved against output.dir
//
// It also rejects two logical mistakes that would otherwise corrupt state:
// two outputs writing to the same file, and an output writing inside the
// snapshot directory (which must stay read-only).
func (c *Config) ResolveOutputs(b *Bootstrap) ([]ResolvedOutput, error) {
	base := c.OutputDir(b)
	snapshots := b.SnapshotsDir()

	resolved := make([]ResolvedOutput, 0, len(c.Configs))
	seen := make(map[string]string, len(c.Configs))

	for _, cf := range c.Configs {
		p := cf.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		p = filepath.Clean(p)

		if prev, dup := seen[p]; dup {
			return nil, fmt.Errorf("configs %q and %q both resolve to %s", prev, cf.Name, p)
		}
		seen[p] = cf.Name

		if within(p, snapshots) {
			return nil, fmt.Errorf("config %q writes to %s, which is inside the read-only snapshot directory", cf.Name, p)
		}

		resolved = append(resolved, ResolvedOutput{Config: cf, Path: p})
	}
	return resolved, nil
}

// within reports whether p is dir itself or lies inside it.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
