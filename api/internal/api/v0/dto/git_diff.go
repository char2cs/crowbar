package dto

import (
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// FileDiffDTO is the wire shape of a single file's parsed diff (04 §3, §4). The
// domain FileDiff is already a clean wire shape carrying stable per-line and
// per-hunk hunkIds from the engine, so the DTO embeds it and the converter only
// guarantees the lines and hunks slices are non-nil for JSON.
type FileDiffDTO struct {
	gitdomain.FileDiff
}

// MultiFileDiffDTO is the wire shape of a commit's diff (04 §3): the commit
// metadata plus the per-file diffs and aggregate counts.
type MultiFileDiffDTO struct {
	gitdomain.MultiFileDiff
}

// FileDiffDTOList converts the working-tree diff (a slice of domain FileDiffs)
// into wire DTOs. It returns a non-nil empty slice for empty input, and ensures
// each file's lines and hunks slices are non-nil, so the envelope's data field
// and nested arrays serialise as [] rather than null.
func FileDiffDTOList(
	diffs []gitdomain.FileDiff,
) []FileDiffDTO {
	out := make([]FileDiffDTO, 0, len(diffs))
	for _, diff := range diffs {
		out = append(out, FileDiffDTO{FileDiff: normalizeFileDiff(diff)})
	}
	return out
}

// MultiFileDiffDTOFrom converts a domain MultiFileDiff (a commit diff) into its
// wire DTO, ensuring the files slice and each file's nested slices are non-nil.
func MultiFileDiffDTOFrom(
	diff gitdomain.MultiFileDiff,
) MultiFileDiffDTO {
	files := make([]gitdomain.FileDiff, 0, len(diff.Files))
	for _, f := range diff.Files {
		files = append(files, normalizeFileDiff(f))
	}
	diff.Files = files
	return MultiFileDiffDTO{MultiFileDiff: diff}
}

func normalizeFileDiff(
	diff gitdomain.FileDiff,
) gitdomain.FileDiff {
	if diff.Lines == nil {
		diff.Lines = []gitdomain.DiffLine{}
	}
	if diff.Hunks == nil {
		diff.Hunks = []gitdomain.Hunk{}
	}
	return diff
}
