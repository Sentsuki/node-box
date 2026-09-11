package build

import (
	"bytes"
	"encoding/json"
	"fmt"

	"node-box/internal/model"
)

// assemble merges the modules listed by cf into a single configuration document.
//
// Merging is shallow and operates on top-level keys only: a module contributes
// whole sing-box sections ("dns", "route", ...), never fragments of one.
func assemble(cf model.ConfigFile, modules map[string]json.RawMessage) (map[string]any, error) {
	doc := make(map[string]any)

	// owner records which module contributed each top-level key, so a conflict
	// can name both sides.
	owner := make(map[string]string)

	for _, name := range cf.Modules {
		raw, ok := modules[name]
		if !ok {
			return nil, fmt.Errorf("module %q was not loaded", name)
		}

		section, err := decodeModule(raw)
		if err != nil {
			return nil, fmt.Errorf("module %q: %w", name, err)
		}

		for key, value := range section {
			if prev, taken := owner[key]; taken {
				// Silently letting the later module win would discard a whole
				// section with no trace. Two modules defining the same section
				// is a configuration error.
				return nil, fmt.Errorf(
					"modules %q and %q both define the top-level key %q; list only one of them, or merge them into a single module",
					prev, name, key)
			}
			owner[key] = name
			doc[key] = value
		}
	}

	return doc, nil
}

// decodeModule parses a module file, requiring a JSON object at the top level.
func decodeModule(raw json.RawMessage) (map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("file is empty")
	}
	if trimmed[0] != '{' {
		return nil, fmt.Errorf("top level must be a JSON object, got %s", jsonKind(trimmed[0]))
	}

	var section map[string]any
	if err := json.Unmarshal(trimmed, &section); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return section, nil
}

// jsonKind names the JSON value a leading byte starts, for error messages.
func jsonKind(first byte) string {
	switch first {
	case '[':
		return "an array"
	case '"':
		return "a string"
	case 't', 'f':
		return "a boolean"
	case 'n':
		return "null"
	default:
		return "a number"
	}
}
