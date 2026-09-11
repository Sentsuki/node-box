package source

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"node-box/internal/fetch"
	"node-box/internal/logx"
)

// Limits on what a tarball is allowed to expand into.
const (
	MaxTarballBytes = 32 << 20 // compressed download
	MaxArchiveBytes = 64 << 20 // total uncompressed
	MaxArchiveFiles = 5000
)

const apiBase = "https://api.github.com"

// GitHub reads the configuration from a GitHub repository.
//
// It fetches whole-repository tarballs rather than individual raw file URLs.
// A tarball is one commit's complete state, which rules out assembling a mix
// of files from different commits, and it is served by the API rather than the
// raw CDN, which can serve minutes-old content after a push.
type GitHub struct {
	client *fetch.Client
	owner  string
	repo   string
	branch string
	token  string

	// mu guards the conditional-request cache, which Resolve reads and writes
	// from both the runner and the poller.
	mu        sync.Mutex
	etag      string
	cachedSHA string
}

// NewGitHub creates a GitHub source. repo is in "owner/name" form.
func NewGitHub(client *fetch.Client, repo, branch, token string) (*GitHub, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return nil, fmt.Errorf("repo %q must be in owner/name form", repo)
	}
	if branch == "" {
		branch = "main"
	}
	return &GitHub{client: client, owner: owner, repo: name, branch: branch, token: token}, nil
}

// Describe names the source.
func (g *GitHub) Describe() string {
	return fmt.Sprintf("github:%s/%s@%s", g.owner, g.repo, g.branch)
}

func (g *GitHub) headers(accept string) map[string]string {
	h := map[string]string{
		"Accept":               accept,
		"X-GitHub-Api-Version": "2022-11-28",
	}
	if g.token != "" {
		h["Authorization"] = "Bearer " + g.token
	}
	return h
}

// Resolve returns the commit sha the branch points at.
//
// The request asks for the bare sha and carries an If-None-Match, so an
// unchanged branch costs one small conditional request. GitHub does not count
// conditional requests that answer 304 against the rate limit, which is what
// makes the fallback poll effectively free.
func (g *GitHub) Resolve(ctx context.Context) (string, error) {
	g.mu.Lock()
	etag, cached := g.etag, g.cachedSHA
	g.mu.Unlock()

	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", apiBase, g.owner, g.repo, g.branch)
	resp, err := g.client.GetWithRetry(ctx, fetch.Request{
		URL:      url,
		Headers:  g.headers("application/vnd.github.sha"),
		ETag:     etag,
		MaxBytes: 4096,
	}, fetch.DefaultRetry)

	if errors.Is(err, fetch.ErrNotModified) {
		if cached == "" {
			// 304 with nothing cached should not happen, but falling back to
			// an unconditional request is better than returning an empty ref.
			return g.resolveUnconditional(ctx)
		}
		logx.Debugf("%s unchanged (%s)", g.Describe(), shortRef(cached))
		return cached, nil
	}
	if err != nil {
		return "", err
	}

	sha := strings.TrimSpace(string(resp.Body))
	if !validRef(sha) {
		return "", fmt.Errorf("unexpected commit sha %q", truncate(sha, 64))
	}

	g.mu.Lock()
	g.etag, g.cachedSHA = resp.ETag, sha
	g.mu.Unlock()
	return sha, nil
}

func (g *GitHub) resolveUnconditional(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", apiBase, g.owner, g.repo, g.branch)
	resp, err := g.client.GetWithRetry(ctx, fetch.Request{
		URL:      url,
		Headers:  g.headers("application/vnd.github.sha"),
		MaxBytes: 4096,
	}, fetch.DefaultRetry)
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(resp.Body))
	if !validRef(sha) {
		return "", fmt.Errorf("unexpected commit sha %q", truncate(sha, 64))
	}
	g.mu.Lock()
	g.etag, g.cachedSHA = resp.ETag, sha
	g.mu.Unlock()
	return sha, nil
}

// Materialize downloads the tarball for ref and extracts it into destDir.
func (g *GitHub) Materialize(ctx context.Context, ref, destDir string) error {
	if !validRef(ref) {
		return fmt.Errorf("invalid ref %q", ref)
	}

	url := fmt.Sprintf("%s/repos/%s/%s/tarball/%s", apiBase, g.owner, g.repo, ref)
	resp, err := g.client.GetWithRetry(ctx, fetch.Request{
		URL:      url,
		Headers:  g.headers("application/vnd.github+json"),
		MaxBytes: MaxTarballBytes,
	}, fetch.DefaultRetry)
	if err != nil {
		return err
	}

	logx.Debugf("downloaded %d bytes of tarball for %s", len(resp.Body), shortRef(ref))
	return extractTarGz(bytes.NewReader(resp.Body), destDir)
}

// extractTarGz unpacks a GitHub repository tarball.
//
// GitHub wraps the repository in a single top-level directory named after the
// owner, repo and commit, which is stripped. Entries are constrained to
// regular files and directories inside destDir: an archive is untrusted input,
// and a crafted one could otherwise write through a symlink or a "../" path.
func extractTarGz(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var totalBytes int64
	var files int

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}

		rel, ok := stripRoot(hdr.Name)
		if !ok {
			continue // the archive's root directory entry
		}
		if !safeRelPath(rel) {
			return fmt.Errorf("archive entry %q escapes the destination", hdr.Name)
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}

		case tar.TypeReg:
			files++
			if files > MaxArchiveFiles {
				return fmt.Errorf("archive contains more than %d files", MaxArchiveFiles)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
			}

			remaining := MaxArchiveBytes - totalBytes
			n, err := writeFile(target, io.LimitReader(tr, remaining+1))
			if err != nil {
				return err
			}
			totalBytes += n
			if totalBytes > MaxArchiveBytes {
				return fmt.Errorf("archive expands to more than %d bytes", MaxArchiveBytes)
			}

		default:
			// Symlinks, hard links, devices and the rest have no place in a
			// configuration repository and are the usual extraction exploit.
			logx.Debugf("skipping archive entry %q of type %d", hdr.Name, hdr.Typeflag)
		}
	}
	return nil
}

// writeFile writes r to path and reports how many bytes were written.
func writeFile(path string, r io.Reader) (int64, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", path, err)
	}
	n, err := io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return n, fmt.Errorf("write %s: %w", path, err)
	}
	return n, nil
}

// stripRoot removes the archive's single top-level directory. It reports false
// for the root entry itself, which has nothing left after stripping.
//
// The name is deliberately not cleaned first. Cleaning "root/../../x" yields
// "../x", and dropping the first component of that would produce the innocent
// looking "x" — laundering a traversal into a valid path. Stripping the raw
// first component instead leaves "../../x" for safeRelPath to reject.
func stripRoot(name string) (string, bool) {
	name = strings.ReplaceAll(name, `\`, "/")
	name = strings.TrimPrefix(name, "./")
	if strings.HasPrefix(name, "/") {
		return "", false
	}
	_, rest, ok := strings.Cut(name, "/")
	if !ok || rest == "" {
		return "", false
	}
	return rest, true
}

// safeRelPath reports whether rel stays inside the destination directory.
func safeRelPath(rel string) bool {
	if rel == "" || path.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return false
	}
	clean := path.Clean(rel)
	return clean != ".." && !strings.HasPrefix(clean, "../")
}

func shortRef(ref string) string { return truncate(ref, 8) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
