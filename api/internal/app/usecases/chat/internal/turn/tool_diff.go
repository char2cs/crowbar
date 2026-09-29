package turn

import (
	"fmt"
	"strings"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const diffContextLines = 3

// editDiff renders one edit's before/after text as a single-hunk unified diff.
// The file itself is never read, so line numbers are relative to the snippet.
func editDiff(path, before, after string) string {
	if path == "" || before == after {
		return ""
	}
	path = strings.TrimLeft(path, "/")
	oldLines, newLines := splitLines(before), splitLines(after)
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix &&
		oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}
	lead, trail := min(prefix, diffContextLines), min(suffix, diffContextLines)
	oldEnd, newEnd := len(oldLines)-suffix, len(newLines)-suffix

	var body strings.Builder
	for _, line := range oldLines[prefix-lead : prefix] {
		body.WriteString(" " + line + "\n")
	}
	for _, line := range oldLines[prefix:oldEnd] {
		body.WriteString("-" + line + "\n")
	}
	for _, line := range newLines[prefix:newEnd] {
		body.WriteString("+" + line + "\n")
	}
	for _, line := range oldLines[oldEnd : oldEnd+trail] {
		body.WriteString(" " + line + "\n")
	}
	oldCount := lead + oldEnd - prefix + trail
	newCount := lead + newEnd - prefix + trail
	return fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ -%s +%s @@\n%s",
		path, path, hunkRange(prefix-lead+1, oldCount), hunkRange(prefix-lead+1, newCount), body.String())
}

// patchDiff renders the provider's own hunks, keeping their true line numbers.
func patchDiff(path string, hunks []engineagents.PatchHunk) string {
	path = strings.TrimLeft(path, "/")
	if path == "" || len(hunks) == 0 {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", path, path)
	for _, hunk := range hunks {
		oldCount, newCount := 0, 0
		for _, line := range hunk.Lines {
			switch {
			case strings.HasPrefix(line, "-"):
				oldCount++
			case strings.HasPrefix(line, "+"):
				newCount++
			default:
				oldCount++
				newCount++
			}
		}
		fmt.Fprintf(&out, "@@ -%s +%s @@\n", hunkRange(hunk.OldStart, oldCount), hunkRange(hunk.NewStart, newCount))
		for _, line := range hunk.Lines {
			out.WriteString(line + "\n")
		}
	}
	return out.String()
}

// hunkRange follows unified-diff convention: an empty side starts one line earlier.
func hunkRange(start, count int) string {
	if count == 0 {
		start--
	}
	return fmt.Sprintf("%d,%d", start, count)
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}
