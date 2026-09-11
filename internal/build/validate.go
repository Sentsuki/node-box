package build

import "fmt"

// groupTypes are outbound types whose members are other outbounds.
var groupTypes = map[string]bool{
	"selector": true,
	"urltest":  true,
}

// validateDocument checks a generated configuration before it is allowed
// anywhere near the disk.
//
// Catching a malformed configuration here rather than at sing-box startup is
// the point: a rejected build leaves the previous, working files in place.
func validateDocument(doc map[string]any) error {
	if len(doc) == 0 {
		return fmt.Errorf("generated configuration is empty; every module contributed nothing")
	}

	for _, section := range []string{"outbounds", "endpoints", "inbounds"} {
		if err := checkArraySection(doc, section); err != nil {
			return err
		}
	}
	tags, err := collectTags(doc)
	if err != nil {
		return err
	}
	return checkReferences(doc, tags)
}

// checkArraySection verifies that a section, if present, is an array of objects.
// A null or non-array value here is what sing-box rejects at startup.
func checkArraySection(doc map[string]any, section string) error {
	raw, ok := doc[section]
	if !ok {
		return nil
	}
	// removeEmptyTopLevel has already deleted nulls and empty arrays, so
	// anything left must be a populated array.
	arr, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("%s must be an array, got %T", section, raw)
	}
	for i, item := range arr {
		if _, ok := item.(map[string]any); !ok {
			return fmt.Errorf("%s[%d] must be an object, got %T", section, i, item)
		}
	}
	return nil
}

// collectTags returns every tag in outbounds and endpoints, rejecting duplicates.
func collectTags(doc map[string]any) (map[string]bool, error) {
	tags := make(map[string]bool)
	for _, section := range []string{"outbounds", "endpoints"} {
		arr, _ := doc[section].([]any)
		for i, raw := range arr {
			obj := raw.(map[string]any)
			tag, ok := obj["tag"].(string)
			if !ok || tag == "" {
				return nil, fmt.Errorf("%s[%d] has no tag", section, i)
			}
			if tags[tag] {
				return nil, fmt.Errorf("duplicate tag %q", tag)
			}
			tags[tag] = true
		}
	}
	return tags, nil
}

// checkReferences verifies that every tag a group or a detour points at exists.
//
// A dangling reference is the failure mode that used to reach sing-box: the
// previous design removed nodes after the fact without touching the selectors
// that still listed them.
func checkReferences(doc map[string]any, tags map[string]bool) error {
	for _, section := range []string{"outbounds", "endpoints"} {
		arr, _ := doc[section].([]any)
		for _, raw := range arr {
			obj := raw.(map[string]any)
			tag, _ := obj["tag"].(string)
			typ, _ := obj["type"].(string)

			if groupTypes[typ] {
				members, ok := obj["outbounds"].([]any)
				if !ok || len(members) == 0 {
					// sing-box refuses to start on an empty group, and a null
					// here is exactly what the previous implementation produced.
					return fmt.Errorf("%s %q has no members", typ, tag)
				}
				for _, m := range members {
					name, ok := m.(string)
					if !ok {
						return fmt.Errorf("%s %q has a non-string member %v", typ, tag, m)
					}
					if !tags[name] {
						return fmt.Errorf("%s %q references %q, which is not defined in this file", typ, tag, name)
					}
				}
			}

			if detour, ok := obj["detour"].(string); ok && detour != "" && !tags[detour] {
				return fmt.Errorf("node %q has detour %q, which is not defined in this file", tag, detour)
			}
		}
	}
	return nil
}
