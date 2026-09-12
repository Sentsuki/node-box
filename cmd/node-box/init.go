package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// starterFiles is the skeleton of a configuration repository.
var starterFiles = map[string]string{
	"config.json": `{
  "output": { "dir": "out" },

  "nodes": {
    "subscriptions": [
      {
        "name": "airport-a",
        "url": "https://example.com/link/REPLACE-ME?clash=1",
        "type": "clash",
        "enable": false,
        "emoji": true,
        "remove_keywords": ["(*人)"]
      }
    ],
    "exclude_keywords": ["过期", "剩余", "官网"]
  },

  "modules": [
    { "name": "log", "file": "modules/log.json" },
    { "name": "dns", "file": "modules/dns.json" },
    { "name": "inbounds", "file": "modules/inbounds.json" },
    { "name": "route", "file": "modules/route.json" }
  ],

  "configs": [
    {
      "name": "main",
      "path": "main.json",
      "modules": ["log", "dns", "inbounds", "route"]
    }
  ],

  "update_schedule": { "type": "interval", "every": "6h" }
}
`,

	"modules/log.json": `{
  "log": {
    "level": "info",
    "timestamp": true
  }
}
`,

	"modules/dns.json": `{
  "dns": {
    "servers": [
      { "tag": "local", "type": "udp", "server": "223.5.5.5" }
    ]
  }
}
`,

	"modules/inbounds.json": `{
  "inbounds": [
    {
      "type": "mixed",
      "tag": "mixed-in",
      "listen": "127.0.0.1",
      "listen_port": 2080
    }
  ]
}
`,

	"modules/route.json": `{
  "route": {
    "rules": [
      { "action": "sniff" }
    ],
    "final": "direct"
  },
  "outbounds": [
    { "type": "direct", "tag": "direct" }
  ]
}
`,

	".github/workflows/notify.yml": `name: Notify node-box

on:
  push:
    branches: [main]
    paths:
      - 'config.json'
      - 'modules/**'
  workflow_dispatch:

concurrency:
  group: notify-node-box
  cancel-in-progress: true

jobs:
  notify:
    runs-on: ubuntu-latest
    steps:
      - name: Ping node-box
        env:
          SECRET: ${{ secrets.NODEBOX_WEBHOOK_SECRET }}
          URL: ${{ secrets.NODEBOX_URL }}
        run: |
          BODY=$(jq -nc --arg ref "$GITHUB_SHA" '{ref:$ref}')
          SIG=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -r | cut -d' ' -f1)
          curl -sS --fail-with-body --max-time 20 \
            -H 'Content-Type: application/json' \
            -H "X-NodeBox-Signature-256: sha256=$SIG" \
            -d "$BODY" \
            "$URL"
`,

	"node-box.json.example": `{
  "log_level": "info",
  "source": {
    "type": "github",
    "repo": "you/node-box-config",
    "branch": "main",
    "token_env": "NODE_BOX_GH_TOKEN",
    "poll_interval": "10m"
  },
  "server": {
    "enabled": true,
    "listen": "127.0.0.1:8788",
    "webhook_secret_env": "NODE_BOX_WEBHOOK_SECRET"
  }
}
`,
}

// cmdInit writes a starter configuration repository.
func cmdInit(_ context.Context, env *env, args []string) error {
	fs := newFlagSet("init")
	env.bind(fs)
	force := fs.Bool("force", false, "overwrite files that already exist")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}

	// Sorted, because ranging a map would print the created and skipped files in
	// a different order every run, and this listing is what the operator reads to
	// check that init did what they expected.
	for _, name := range slices.Sorted(maps.Keys(starterFiles)) {
		content := starterFiles[name]
		path := filepath.Join(dir, filepath.FromSlash(name))
		if !*force {
			if _, err := os.Stat(path); err == nil {
				fmt.Printf("skip   %s (already exists)\n", name)
				continue
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if err := writeStarter(path, content); err != nil {
			return err
		}
		fmt.Printf("create %s\n", name)
	}

	fmt.Printf(`
Next steps:
  1. Edit config.json: replace the example subscription URL with a real one,
     then set its "enable" to true. It ships disabled so the first
     "validate" succeeds against the placeholder.
  2. Commit this directory to a private GitHub repository.
  3. Copy node-box.json.example to the node-box host as node-box.json and
     fill in the repository name.
  4. On the host, put the GitHub token and the webhook secret in .env, then run
     "%s validate" followed by "%s update".
`, appName, appName)
	return nil
}

// starterPerm keeps generated repository files readable. Unlike generated
// configuration, nothing written by init contains a credential: the secrets go
// in .env on the host, and node-box.json.example carries only variable names.
const starterPerm = 0o644

func writeStarter(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), starterPerm)
}
