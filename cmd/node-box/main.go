// Command node-box generates sing-box configuration from a configuration
// repository and a set of subscriptions.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"node-box/internal/logx"
	"node-box/internal/model"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

const appName = "node-box"

// ConfigPathEnv names the environment variable holding the bootstrap config path.
const ConfigPathEnv = "NODE_BOX_CONFIG"

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		logx.Errorf("%v", err)
		os.Exit(1)
	}
}

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, env *env, args []string) error
}

func commands() []command {
	return []command{
		{"run", "run continuously: schedule, webhook and fallback poll", cmdRun},
		{"update", "perform one complete update and exit", cmdUpdate},
		{"pull", "refresh the snapshot without generating output", cmdPull},
		{"build", "assemble without writing; --diff shows what would change", cmdBuild},
		{"validate", "check that a snapshot assembles into valid configuration", cmdValidate},
		{"rollback", "regenerate output from the last-good snapshot", cmdRollback},
		{"status", "report the current ref, last update and last error", cmdStatus},
		{"init", "write a starter configuration repository", cmdInit},
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}

	switch args[0] {
	case "-h", "--help", "help":
		usage()
		return nil
	case "-v", "--version", "version":
		fmt.Printf("%s %s\n", appName, version)
		return nil
	}

	name := args[0]
	for _, c := range commands() {
		if c.name != name {
			continue
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		env := &env{}
		return c.run(ctx, env, args[1:])
	}

	usage()
	return fmt.Errorf("unknown command %q", name)
}

func usage() {
	fmt.Printf("%s %s\n\nUsage:\n  %s <command> [flags]\n\nCommands:\n", appName, version, appName)
	for _, c := range commands() {
		fmt.Printf("  %-9s %s\n", c.name, c.summary)
	}
	fmt.Printf(`
Global flags:
  --config <path>   bootstrap configuration (default: $%s, else ./node-box.json)
  --log-level <lvl> silent, error, warn, info or debug

Run "%s <command> -h" for command flags.
`, ConfigPathEnv, appName)
}

// env holds what every command resolves from its flags.
type env struct {
	configPath string
	logLevel   string
}

// bind registers the global flags on a command's flag set.
func (e *env) bind(fs *flag.FlagSet) {
	fs.StringVar(&e.configPath, "config", "", "path to the bootstrap configuration")
	fs.StringVar(&e.logLevel, "log-level", "", "override the configured log level")
}

// loadBootstrap resolves the bootstrap path, loads it and applies the log level.
//
// Resolution order is flag, then environment, then the default next to the
// binary. Unlike the previous implementation there is no case where an
// explicit path loses to the environment.
func (e *env) loadBootstrap() (*model.Bootstrap, error) {
	path := e.configPath
	if path == "" {
		path = os.Getenv(ConfigPathEnv)
	}
	if path == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("locate the executable: %w", err)
		}
		path = filepath.Join(filepath.Dir(exe), "node-box.json")
	}

	boot, err := model.LoadBootstrap(path)
	if err != nil {
		return nil, err
	}

	level := boot.LogLevel
	if e.logLevel != "" {
		level = e.logLevel
	}
	parsed, err := logx.ParseLevel(level)
	if err != nil {
		return nil, err
	}
	logx.SetLevel(parsed)

	logx.Debugf("bootstrap configuration: %s", path)
	logx.Debugf("state root: %s", boot.Root)
	return boot, nil
}

// newFlagSet builds a flag set that reports errors through the command.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(appName+" "+name, flag.ContinueOnError)
	return fs
}
