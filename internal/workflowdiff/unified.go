package workflowdiff

import (
	"fmt"
	"strings"
)

// Unified renders one replacement hunk between the first and last changed
// lines, with three context lines at either end. It is deliberately not a
// minimal edit script: linear work and memory also handle large pinned data.
func Unified(from, to []byte, fromLabel, toLabel string) string {
	if string(from) == string(to) {
		return ""
	}
	a, b := strings.Split(string(from), "\n"), strings.Split(string(to), "\n")
	start := 0
	for start < min(len(a), len(b)) && a[start] == b[start] {
		start++
	}
	endA, endB := len(a), len(b)
	for endA > start && endB > start && a[endA-1] == b[endB-1] {
		endA--
		endB--
	}
	contextStart := max(0, start-3)
	suffix := min(3, len(a)-endA)
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n@@ -%d,%d +%d,%d @@\n", fromLabel, toLabel,
		contextStart+1, endA+suffix-contextStart, contextStart+1, endB+suffix-contextStart)
	for _, line := range a[contextStart:start] {
		fmt.Fprintf(&out, " %s\n", line)
	}
	for _, line := range a[start:endA] {
		fmt.Fprintf(&out, "-%s\n", line)
	}
	for _, line := range b[start:endB] {
		fmt.Fprintf(&out, "+%s\n", line)
	}
	for _, line := range a[endA : endA+suffix] {
		fmt.Fprintf(&out, " %s\n", line)
	}
	return out.String()
}
