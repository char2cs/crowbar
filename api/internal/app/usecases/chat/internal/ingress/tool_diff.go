package ingress

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const diffContextLines = 3

func toolStartDiff(tool *engineagents.ToolEvent) string {
	if tool.Diff != "" {
		return tool.Diff
	}
	return editDiff(tool.Target, tool.Cwd, tool.EditBefore, tool.EditAfter)
}

func toolResultDiff(tool *engineagents.ToolEvent) string {
	if tool.Diff != "" {
		return tool.Diff
	}
	return patchDiff(tool.Target, tool.Cwd, tool.Patch)
}

// toolTarget is the call's target; a read or edit's is its file, named from
// the session's directory.
func toolTarget(tool *engineagents.ToolEvent) string {
	if tool.Kind == ledger.ToolKindRead || tool.Kind == ledger.ToolKindEdit {
		return sessionPath(tool.Target, tool.Cwd)
	}
	return tool.Target
}

// sessionPath is path relative to cwd when it lies inside it, else path as given.
func sessionPath(path, cwd string) string {
	if rel, err := filepath.Rel(cwd, path); err == nil && cwd != "" && filepath.IsAbs(path) && filepath.IsLocal(rel) {
		return filepath.ToSlash(rel)
	}
	return path
}

// editDiff renders one edit's before/after text as a single-hunk unified diff.
// The file itself is never read, so line numbers are relative to the snippet;
// no text before is a new file's.
func editDiff(path, cwd, before, after string) string {
	if path == "" || before == after {
		return ""
	}
	// A file outside the session keeps its leading slash (a//abs): it is not one inside it.
	path = sessionPath(path, cwd)
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
	from := "a/" + path
	if before == "" {
		from = "/dev/null"
	}
	return fmt.Sprintf("--- %s\n+++ b/%s\n@@ -%s +%s @@\n%s",
		from, path, hunkRange(prefix-lead+1, oldCount), hunkRange(prefix-lead+1, newCount), body.String())
}

// patchDiff renders the provider's own hunks, keeping their true line numbers.
func patchDiff(path, cwd string, hunks []engineagents.PatchHunk) string {
	path = sessionPath(path, cwd)
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
