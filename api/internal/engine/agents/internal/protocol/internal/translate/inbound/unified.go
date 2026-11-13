package inbound

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// unifiedHunks splits a unified diff into its hunks; file headers and
// "\ No newline" markers are not lines of any hunk.
func unifiedHunks(diff string) []models.PatchHunk {
	var out []models.PatchHunk
	for _, line := range strings.Split(diff, "\n") {
		if m := hunkHeader.FindStringSubmatch(line); m != nil {
			oldStart, _ := strconv.Atoi(m[1])
			newStart, _ := strconv.Atoi(m[2])
			out = append(out, models.PatchHunk{OldStart: oldStart, NewStart: newStart})
			continue
		}
		if len(out) == 0 || line == "" || line[0] == '\\' {
			continue
		}
		last := &out[len(out)-1]
		last.Lines = append(last.Lines, line)
	}
	return out
}

func buildPatch(patch *spec.PatchSpec, decoded map[string]any) []models.PatchHunk {
	if patch == nil {
		return nil
	}
	if patch.Unified != "" {
		return unifiedHunks(mapping.String(decoded, mapping.Paths(patch.Unified)))
	}
	var out []models.PatchHunk
	for _, row := range mapping.Objects(decoded, mapping.Paths(patch.Items)) {
		oldStart, _ := mapping.Int(row, mapping.Paths(patch.OldStart))
		newStart, _ := mapping.Int(row, mapping.Paths(patch.NewStart))
		lines := mapping.Strings(row, mapping.Paths(patch.Lines))
		if len(lines) == 0 {
			continue
		}
		out = append(out, models.PatchHunk{OldStart: oldStart, NewStart: newStart, Lines: lines})
	}
	return out
}
