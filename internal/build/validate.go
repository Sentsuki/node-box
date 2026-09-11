package build

import (
	"fmt"
)

// validateDocument checks a generated configuration before it is allowed
// anywhere near the disk.
//
// Catching a malformed configuration here rather than at sing-box startup is
// the point: a rejected build leaves the previous, working files in place.
//
// TODO(outbounds): once node injection is designed, this must also check that
//   - every selector and urltest has a non-empty outbounds array
//   - every tag a selector references exists in the same document
//   - tags are unique across outbounds and endpoints
//   - the top-level outbounds array is non-empty
func validateDocument(doc map[string]any) error {
	if len(doc) == 0 {
		return fmt.Errorf("generated configuration is empty; every module contributed nothing")
	}

	for _, section := range []string{"outbounds", "endpoints", "inbounds"} {
		if err := checkArraySection(doc, section); err != nil {
			return err
		}
	}
	return nil
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
