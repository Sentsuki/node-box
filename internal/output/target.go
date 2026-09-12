package output

import (
	"fmt"
	"path/filepath"

	"node-box/internal/fsx"
	"node-box/internal/model"
)

// Target pairs one configs[] entry with the absolute file it writes to.
type Target struct {
	Config model.ConfigFile
	Path   string
}

// BaseDir returns the absolute base directory for relative output paths.
//
// A relative output.dir is resolved against the bootstrap root; an empty one
// defaults to <root>/out.
func BaseDir(c *model.Config, b *model.Bootstrap) string {
	if c.Output == nil || c.Output.Dir == "" {
		return b.DefaultOutputDir()
	}
	dir := c.Output.Dir
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(b.Root, dir)
	}
	return filepath.Clean(dir)
}

// Resolve turns every configs[] entry into an absolute destination.
//
// Resolution lives here rather than on Config because it needs the bootstrap
// configuration to know where the root is. A method on Config taking a
// *Bootstrap made the repository's own schema depend on the local one, which is
// backwards: the repository is meant to describe itself and nothing else.
//
// Resolution rules:
//   - an absolute path is used as-is and ignores output.dir
//   - a relative path is resolved against output.dir
//
// It also rejects two logical mistakes that would otherwise corrupt state:
// two outputs writing to the same file, and an output writing inside the
// snapshot directory (which must stay read-only).
func Resolve(c *model.Config, b *model.Bootstrap) ([]Target, error) {
	base := BaseDir(c, b)
	snapshots := b.SnapshotsDir()

	targets := make([]Target, 0, len(c.Configs))
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

		if fsx.Within(p, snapshots) {
			return nil, fmt.Errorf("config %q writes to %s, which is inside the read-only snapshot directory", cf.Name, p)
		}

		targets = append(targets, Target{Config: cf, Path: p})
	}
	return targets, nil
}
