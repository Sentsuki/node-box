package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Source type names.
const (
	SourceGitHub = "github"
	SourceLocal  = "local"
)

// Defaults applied when the bootstrap file omits a field.
const (
	DefaultBranch       = "main"
	DefaultPollInterval = Duration(10 * time.Minute)
	DefaultListen       = "127.0.0.1:8788"
)

// Bootstrap is the local configuration file (node-box.json). It describes where
// to get the real configuration from and how this process is wired up. It never
// lives in the config repository, because it is what tells node-box how to reach
// that repository in the first place.
type Bootstrap struct {
	Root     string        `json:"root,omitempty"`
	LogLevel string        `json:"log_level,omitempty"`
	Source   *SourceConfig `json:"source"`
	Server   *ServerConfig `json:"server,omitempty"`
	Proxy    *ProxyConfig  `json:"proxy,omitempty"`
}

// SourceConfig describes where the configuration snapshot comes from.
type SourceConfig struct {
	Type         string   `json:"type"`
	Repo         string   `json:"repo,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	TokenEnv     string   `json:"token_env,omitempty"`
	PollInterval Duration `json:"poll_interval,omitempty"`
	Dir          string   `json:"dir,omitempty"`
}

// ServerConfig describes the built-in HTTP server used for webhooks.
type ServerConfig struct {
	Enabled          bool   `json:"enabled"`
	Listen           string `json:"listen,omitempty"`
	WebhookSecretEnv string `json:"webhook_secret_env,omitempty"`
}

// ProxyConfig configures an outbound proxy for all HTTP fetching (both the
// config source and subscriptions).
type ProxyConfig struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// LoadBootstrap reads, parses, defaults and validates node-box.json.
func LoadBootstrap(path string) (*Bootstrap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bootstrap config %s: %w", path, err)
	}

	var b Bootstrap
	if err := strictUnmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse bootstrap config %s: %w", path, err)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve bootstrap config path: %w", err)
	}
	b.applyDefaults(filepath.Dir(abs))

	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("invalid bootstrap config %s: %w", path, err)
	}
	return &b, nil
}

// applyDefaults fills in omitted fields. baseDir is the directory holding the
// bootstrap file, used as the default root.
func (b *Bootstrap) applyDefaults(baseDir string) {
	if b.Root == "" {
		b.Root = baseDir
	} else if !filepath.IsAbs(b.Root) {
		b.Root = filepath.Join(baseDir, b.Root)
	}
	b.Root = filepath.Clean(b.Root)

	if b.LogLevel == "" {
		b.LogLevel = "info"
	}
	if b.Source != nil {
		if b.Source.Branch == "" {
			b.Source.Branch = DefaultBranch
		}
		if b.Source.PollInterval.IsZero() && b.Source.Type == SourceGitHub {
			b.Source.PollInterval = DefaultPollInterval
		}
		if b.Source.Dir != "" && !filepath.IsAbs(b.Source.Dir) {
			b.Source.Dir = filepath.Join(baseDir, b.Source.Dir)
		}
	}
	if b.Server != nil && b.Server.Listen == "" {
		b.Server.Listen = DefaultListen
	}
}

// Validate checks the bootstrap configuration for structural errors.
func (b *Bootstrap) Validate() error {
	if b.Source == nil {
		return fmt.Errorf("source is required")
	}
	if err := b.Source.validate(); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if b.Server != nil && b.Server.Enabled {
		if err := b.Server.validate(); err != nil {
			return fmt.Errorf("server: %w", err)
		}
	}
	if b.Proxy != nil {
		if err := b.Proxy.validate(); err != nil {
			return fmt.Errorf("proxy: %w", err)
		}
	}
	return nil
}

func (s *SourceConfig) validate() error {
	switch s.Type {
	case SourceGitHub:
		if s.Repo == "" {
			return fmt.Errorf("repo is required when type is %q", SourceGitHub)
		}
		owner, name, ok := strings.Cut(s.Repo, "/")
		if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
			return fmt.Errorf("repo %q must be in owner/name form", s.Repo)
		}
		if s.TokenEnv == "" {
			return fmt.Errorf("token_env is required when type is %q", SourceGitHub)
		}
		if s.Dir != "" {
			return fmt.Errorf("dir is only valid when type is %q", SourceLocal)
		}
		if s.PollInterval < 0 {
			return fmt.Errorf("poll_interval must not be negative")
		}
	case SourceLocal:
		if s.Dir == "" {
			return fmt.Errorf("dir is required when type is %q", SourceLocal)
		}
		if s.Repo != "" || s.TokenEnv != "" {
			return fmt.Errorf("repo and token_env are only valid when type is %q", SourceGitHub)
		}
	case "":
		return fmt.Errorf("type is required (%q or %q)", SourceGitHub, SourceLocal)
	default:
		return fmt.Errorf("unknown type %q (want %q or %q)", s.Type, SourceGitHub, SourceLocal)
	}
	return nil
}

func (s *ServerConfig) validate() error {
	if s.WebhookSecretEnv == "" {
		return fmt.Errorf("webhook_secret_env is required when the server is enabled")
	}
	host, port, err := net.SplitHostPort(s.Listen)
	if err != nil {
		return fmt.Errorf("listen %q must be host:port: %w", s.Listen, err)
	}
	if port == "" {
		return fmt.Errorf("listen %q is missing a port", s.Listen)
	}
	// Binding a public interface is almost never intended: TLS belongs in a
	// reverse proxy. Warn loudly by refusing the obvious mistakes.
	if host == "" || host == "0.0.0.0" || host == "::" {
		return fmt.Errorf("listen %q binds all interfaces; bind a loopback address and terminate TLS in a reverse proxy", s.Listen)
	}
	return nil
}

func (p *ProxyConfig) validate() error {
	if p.Host == "" {
		return fmt.Errorf("host cannot be empty")
	}
	if p.Port <= 0 || p.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", p.Port)
	}
	valid := []string{"http", "https", "socks5"}
	if !slices.Contains(valid, strings.ToLower(p.Type)) {
		return fmt.Errorf("unknown type %q (want one of %v)", p.Type, valid)
	}
	return nil
}

// SnapshotsDir is where immutable config snapshots are stored.
func (b *Bootstrap) SnapshotsDir() string { return filepath.Join(b.Root, "snapshots") }

// StateDir holds state.json.
func (b *Bootstrap) StateDir() string { return filepath.Join(b.Root, "state") }

// StateFile is the path of the persisted run state.
func (b *Bootstrap) StateFile() string { return filepath.Join(b.StateDir(), "state.json") }

// LockFile is the single-writer lock guarding this root. Everything that
// mutates state below Root — snapshots, pointers, state.json and the generated
// files — is serialised through it, so a daemon and a one-shot CLI invocation
// cannot walk over each other.
func (b *Bootstrap) LockFile() string { return filepath.Join(b.StateDir(), "update.lock") }

// DefaultOutputDir is used when the repo config does not set output.dir.
func (b *Bootstrap) DefaultOutputDir() string { return filepath.Join(b.Root, "out") }

// strictUnmarshal decodes JSON and rejects unknown fields, so a typo in a key
// name fails loudly instead of being silently ignored.
func strictUnmarshal(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	// Reject trailing content after the top-level value.
	var extra json.RawMessage
	if err := dec.Decode(&extra); err == nil {
		return fmt.Errorf("unexpected trailing content after the JSON object")
	}
	return nil
}
