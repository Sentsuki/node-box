package main

import (
	"fmt"
	"strings"
)

// maxDiffLines bounds the quadratic part of the diff. Generated configuration
// is small, and a pathological input should print a summary rather than
// allocate an enormous table.
const maxDiffLines = 2000

// printDiff writes a line-oriented diff of two texts.
func printDiff(oldText, newText string) {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	// Identical head and tail are the overwhelming majority of two versions of
	// the same config; trimming them first keeps the real work tiny.
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix &&
		oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}

	oldMid := oldLines[prefix : len(oldLines)-suffix]
	newMid := newLines[prefix : len(newLines)-suffix]

	if len(oldMid) > maxDiffLines || len(newMid) > maxDiffLines {
		fmt.Printf("    (%d lines removed, %d added; too large to show)\n", len(oldMid), len(newMid))
		return
	}

	for _, line := range diffLines(oldMid, newMid) {
		fmt.Printf("    %s\n", line)
	}
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// diffLines returns the changed lines prefixed with "-" and "+", using a
// longest-common-subsequence walk.
func diffLines(a, b []string) []string {
	// lcs[i][j] is the length of the longest common subsequence of a[i:] and b[j:].
	lcs := make([][]int32, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	var out []string
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, "- "+a[i])
			i++
		default:
			out = append(out, "+ "+b[j])
			j++
		}
	}
	for ; i < len(a); i++ {
		out = append(out, "- "+a[i])
	}
	for ; j < len(b); j++ {
		out = append(out, "+ "+b[j])
	}
	return out
}
