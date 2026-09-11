// Package build assembles generated sing-box configuration from a set of
// modules and subscription nodes.
//
// Build is a pure function: it performs no disk or network I/O and reads no
// global state. Everything it needs arrives in Input, and everything it
// produces is returned as in-memory files. That is what makes the output
// testable, diffable before it is written, and safe to discard when a later
// validation step rejects it.
package build

import (
	"encoding/json"
	"fmt"
	"slices"

	"node-box/internal/logx"
	"node-box/internal/model"
	"node-box/internal/output"
	"node-box/internal/subscription"
)

// Input is everything needed to produce the output files.
type Input struct {
	// Config is the validated repository configuration.
	Config *model.Config

	// Modules maps module name to its raw JSON content.
	Modules map[string]json.RawMessage

	// Nodes maps subscription name to its parsed nodes, already named and
	// filtered by the subscription layer.
	Nodes map[string][]subscription.Node

	// Outputs are the resolved absolute destinations for Config.Configs.
	Outputs []model.ResolvedOutput

	// Injector places subscription nodes into outbounds and endpoints.
	// Nil means PassthroughInjector.
	Injector NodeInjector
}

// Build assembles every configured output file.
//
// An error from any single output aborts the whole build: a partial set of
// files would leave the generated configurations inconsistent with each other.
func Build(in Input) ([]output.File, error) {
	if in.Config == nil {
		return nil, fmt.Errorf("config is required")
	}
	injector := in.Injector
	if injector == nil {
		injector = PassthroughInjector{}
	}

	files := make([]output.File, 0, len(in.Outputs))
	for _, out := range in.Outputs {
		f, err := buildOne(out, in, injector)
		if err != nil {
			return nil, fmt.Errorf("config %q: %w", out.Config.Name, err)
		}
		files = append(files, f)
	}
	return files, nil
}

func buildOne(out model.ResolvedOutput, in Input, injector NodeInjector) (output.File, error) {
	doc, err := assemble(out.Config, in.Modules)
	if err != nil {
		return output.File{}, err
	}

	if err := injector.Inject(doc, out.Config, in.Nodes); err != nil {
		return output.File{}, fmt.Errorf("inject nodes: %w", err)
	}

	if removed := removeEmptyTopLevel(doc); len(removed) > 0 {
		logx.Debugf("config %q: dropped empty sections %v", out.Config.Name, removed)
	}

	if err := validateDocument(doc); err != nil {
		return output.File{}, err
	}

	content, err := encode(doc)
	if err != nil {
		return output.File{}, err
	}

	return output.File{
		Name:    out.Config.Name,
		Path:    out.Path,
		Content: content,
	}, nil
}

// encode serialises the document deterministically.
//
// encoding/json sorts map keys, so identical input always produces identical
// bytes. That is what lets the writer skip files whose content has not changed.
func encode(doc map[string]any) ([]byte, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return append(data, '\n'), nil
}

// removeEmptyTopLevel deletes top-level keys whose value is null, an empty
// array or an empty object, and returns the names removed.
//
// This is deliberately a general rule rather than a list of known sing-box
// sections: a hardcoded list would be a second source of truth that drifts
// from what modules actually contain.
func removeEmptyTopLevel(doc map[string]any) []string {
	var removed []string
	for key, value := range doc {
		empty := false
		switch v := value.(type) {
		case nil:
			empty = true
		case []any:
			empty = len(v) == 0
		case map[string]any:
			empty = len(v) == 0
		}
		if empty {
			delete(doc, key)
			removed = append(removed, key)
		}
	}
	// Map iteration order is random; sort so logs and tests are stable.
	slices.Sort(removed)
	return removed
}
