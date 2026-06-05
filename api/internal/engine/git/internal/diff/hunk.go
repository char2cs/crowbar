package diff

import (
	"strconv"
	"strings"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func buildHunkLines(
	rawLines []string,
	filePath string,
) []gitdomain.DiffLine {
	if len(rawLines) == 0 {
		return nil
	}
	header := rawLines[0]
	bodyLines := rawLines[1:]

	oldLine, newLine := parseHunkHeader(header)

	bodyText := buildHunkBody(bodyLines)
	hunkID := HunkID(filePath, bodyText)

	var result []gitdomain.DiffLine

	headerLine := gitdomain.DiffLine{
		LineType: gitdomain.DiffLineHeader,
		Content:  header,
	}
	result = append(result, headerLine)

	for _, raw := range bodyLines {
		if raw == "" || raw == "\\ No newline at end of file" {
			continue
		}
		dl := buildDiffLine(raw, hunkID, &oldLine, &newLine)
		result = append(result, dl)
	}

	return result
}

func buildHunkBody(
	bodyLines []string,
) string {
	var parts []string
	for _, line := range bodyLines {
		if line == "" || line == "\\ No newline at end of file" {
			continue
		}
		ch := line[0]
		if ch == '+' || ch == '-' || ch == ' ' {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, "\n")
}

func buildDiffLine(
	raw string,
	hunkID string,
	oldLine *int,
	newLine *int,
) gitdomain.DiffLine {
	if len(raw) == 0 {
		return gitdomain.DiffLine{}
	}
	ch := raw[0]
	content := raw[1:]

	switch ch {
	case '+':
		n := *newLine
		*newLine++
		return gitdomain.DiffLine{
			LineType:      gitdomain.DiffLineAdded,
			Content:       content,
			NewLineNumber: &n,
			HunkID:        hunkID,
		}
	case '-':
		o := *oldLine
		*oldLine++
		return gitdomain.DiffLine{
			LineType:      gitdomain.DiffLineRemoved,
			Content:       content,
			OldLineNumber: &o,
			HunkID:        hunkID,
		}
	case ' ':
		o := *oldLine
		n := *newLine
		*oldLine++
		*newLine++
		return gitdomain.DiffLine{
			LineType:      gitdomain.DiffLineContext,
			Content:       content,
			OldLineNumber: &o,
			NewLineNumber: &n,
		}
	}
	return gitdomain.DiffLine{
		LineType: gitdomain.DiffLineContext,
		Content:  raw,
	}
}

func parseHunkHeader(
	header string,
) (int, int) {
	start := strings.Index(header, "-")
	if start < 0 {
		return 1, 1
	}
	rest := header[start+1:]
	end := strings.IndexAny(rest, " ,")
	if end < 0 {
		end = len(rest)
	}
	oldStart, _ := strconv.Atoi(rest[:end])

	plusIdx := strings.Index(header, "+")
	if plusIdx < 0 {
		return oldStart, 1
	}
	rest2 := header[plusIdx+1:]
	end2 := strings.IndexAny(rest2, " ,")
	if end2 < 0 {
		end2 = len(rest2)
	}
	newStart, _ := strconv.Atoi(rest2[:end2])

	return oldStart, newStart
}

func buildHunks(
	lines []gitdomain.DiffLine,
) []gitdomain.Hunk {
	var hunks []gitdomain.Hunk
	var currentHunkID string
	currentStart := -1

	for i, line := range lines {
		if line.LineType == gitdomain.DiffLineHeader {
			if currentHunkID != "" {
				hunks[len(hunks)-1].EndLine = i - 1
			}
			currentHunkID = ""
			currentStart = i
			continue
		}
		// Start a new hunk when HunkID changes or when we first see a
		// non-header line after a @@ header (currentStart >= 0 means we
		// just passed a header and haven't started a hunk yet).
		if line.HunkID != "" && (line.HunkID != currentHunkID || currentHunkID == "") {
			currentHunkID = line.HunkID
			hunks = append(hunks, gitdomain.Hunk{
				HunkID:    currentHunkID,
				Header:    headerForHunk(lines, currentStart),
				StartLine: currentStart,
				EndLine:   i,
			})
			continue
		}
		if len(hunks) > 0 {
			hunks[len(hunks)-1].EndLine = i
		}
	}
	return hunks
}

func headerForHunk(
	lines []gitdomain.DiffLine,
	headerIdx int,
) string {
	if headerIdx < 0 || headerIdx >= len(lines) {
		return ""
	}
	if lines[headerIdx].LineType == gitdomain.DiffLineHeader {
		return lines[headerIdx].Content
	}
	return ""
}
