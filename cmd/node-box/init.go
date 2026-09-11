package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
        "enable": true,
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

	for name, content := range starterFiles {
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
  1. Edit config.json: replace the example subscription with a real one.
  2. Commit this directory to a private GitHub repository.
  3. Copy node-box.json.example to the node-box host as node-box.json and
     fill in the repository name.
  4. On the host, put the GitHub token and the webhook secret in .env, then run
     "%s validate" followed by "%s update".
`, appName, appName)
	return nil
}

func writeStarter(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), starterPerm(path))
}

// starterPerm keeps generated repository files readable; nothing written here
// contains a secret.
func starterPerm(string) fs.FileMode { return 0o644 }
