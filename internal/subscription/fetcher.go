package subscription

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"node-box/internal/fetch"
	"node-box/internal/logx"
	"node-box/internal/model"
	"node-box/internal/node"
)

// DefaultParallel bounds concurrent subscription fetches. Fetching serially
// meant one dead URL stalled the whole run for the length of its retry budget.
const DefaultParallel = 4

// Fetcher retrieves and prepares nodes for every enabled subscription.
type Fetcher struct {
	client *fetch.Client
	// baseDir resolves repository-relative subscription paths, i.e. the
	// snapshot directory.
	baseDir  string
	parallel int
}

// NewFetcher creates a Fetcher.
func NewFetcher(client *fetch.Client, baseDir string) *Fetcher {
	return &Fetcher{client: client, baseDir: baseDir, parallel: DefaultParallel}
}

// FetchAll fetches every enabled subscription concurrently and returns its
// nodes keyed by subscription name.
//
// A subscription that fails is logged and skipped. An error is returned only
// when every enabled subscription failed, because that is the case where
// continuing would regenerate configurations with no nodes in them.
func (f *Fetcher) FetchAll(ctx context.Context, cfg *model.Config) (map[string][]node.Node, error) {
	var enabled []model.Subscription
	for _, s := range cfg.Nodes.Subscriptions {
		if s.Enable {
			enabled = append(enabled, s)
		}
	}
	if len(enabled) == 0 {
		logx.Debugf("no enabled subscriptions")
		return map[string][]node.Node{}, nil
	}

	naming := namingRules{
		exclude:   cfg.Nodes.ExcludeKeywords,
		defaultUA: cfg.UserAgent,
		emoji:     newEmojiTable(cfg.Nodes.EmojiOverrides),
	}

	type result struct {
		nodes []node.Node
		err   error
	}
	results := make([]result, len(enabled))

	sem := make(chan struct{}, f.parallel)
	var wg sync.WaitGroup
	for i, sub := range enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[i] = result{err: ctx.Err()}
				return
			}
			nodes, err := f.fetchOne(ctx, sub, naming)
			results[i] = result{nodes: nodes, err: err}
		}()
	}
	wg.Wait()

	out := make(map[string][]node.Node, len(enabled))
	var failed []string
	for i, sub := range enabled {
		if err := results[i].err; err != nil {
			logx.Errorf("subscription %q: %v", sub.Name, err)
			failed = append(failed, sub.Name)
			continue
		}
		out[sub.Name] = results[i].nodes
		logx.Debugf("subscription %q: %d nodes", sub.Name, len(results[i].nodes))
	}

	switch {
	case len(failed) == len(enabled):
		return nil, fmt.Errorf("all %d subscriptions failed: %s", len(failed), strings.Join(failed, ", "))
	case len(failed) > 0:
		logx.Warnf("%d of %d subscriptions failed (%s); continuing with the rest",
			len(failed), len(enabled), strings.Join(failed, ", "))
	default:
		logx.Infof("fetched %d subscriptions", len(enabled))
	}
	return out, nil
}

// namingRules is the configuration-wide half of the naming pipeline, prepared
// once per run rather than per subscription.
type namingRules struct {
	exclude   []string
	defaultUA string
	emoji     emojiTable
}

// fetchOne retrieves one subscription and applies its naming rules.
func (f *Fetcher) fetchOne(ctx context.Context, sub model.Subscription, naming namingRules) ([]node.Node, error) {
	data, err := f.read(ctx, sub, naming.defaultUA)
	if err != nil {
		return nil, err
	}

	processor, err := ProcessorFor(sub.Type)
	if err != nil {
		return nil, err
	}
	nodes, err := processor.Process(data)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	// Keyword removal runs before emoji handling so the keywords match against
	// names that still have their original shape.
	if len(sub.RemoveKeywords) > 0 {
		nodes = removeKeywords(nodes, sub.RemoveKeywords)
	}
	if sub.Emoji != nil {
		if *sub.Emoji {
			nodes = assignTagEmoji(nodes, naming.emoji)
		} else {
			nodes = stripTagEmoji(nodes)
		}
	}

	nodes = prefixTags(nodes, sub.Name)

	// Globally excluded nodes are dropped last, so the keywords match the final
	// tag and the dropped nodes take no part in any later step.
	before := len(nodes)
	nodes = dropExcluded(nodes, naming.exclude)
	if dropped := before - len(nodes); dropped > 0 {
		logx.Debugf("subscription %q: excluded %d nodes", sub.Name, dropped)
	}

	return nodes, nil
}

// read returns the raw subscription payload from its URL or its file.
func (f *Fetcher) read(ctx context.Context, sub model.Subscription, defaultUA string) ([]byte, error) {
	if sub.Path != "" {
		path := filepath.Join(f.baseDir, filepath.FromSlash(sub.Path))
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		return data, nil
	}

	ua := sub.UserAgent
	if ua == "" {
		ua = defaultUA
	}
	resp, err := f.client.GetWithRetry(ctx, fetch.Request{URL: sub.URL, UserAgent: ua}, fetch.DefaultRetry)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
