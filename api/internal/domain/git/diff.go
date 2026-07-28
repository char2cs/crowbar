package git

import (
	"encoding/json"
	"time"
)

// DiffLine is one line in a FileDiff's flat rendering. Changed lines carry
// hunkId linking them to their Hunk (04 §4).
type DiffLine struct {
	LineType      DiffLineType `json:"line_type"`
	Content       string       `json:"content"`
	OldLineNumber *int         `json:"old_line_number,omitempty"`
	NewLineNumber *int         `json:"new_line_number,omitempty"`
	HunkID        string       `json:"hunkId,omitempty"`
}

// DiffLineType classifies a single line in a unified diff (04 §3).
type DiffLineType string

const (
	DiffLineAdded   DiffLineType = "added"
	DiffLineRemoved DiffLineType = "removed"
	DiffLineContext DiffLineType = "context"
	DiffLineHeader  DiffLineType = "header"
)

// FileDiff is the parsed diff for a single file (04 §3, §4).
type FileDiff struct {
	FilePath      string     `json:"file_path"`
	OldPath       string     `json:"old_path,omitempty"`
	NewPath       string     `json:"new_path,omitempty"`
	IsNew         bool       `json:"is_new"`
	IsDeleted     bool       `json:"is_deleted"`
	IsRenamed     bool       `json:"is_renamed"`
	IsBinary      bool       `json:"is_binary,omitempty"`
	IsImage       bool       `json:"is_image,omitempty"`
	OldBlobBase64 string     `json:"old_blob_base64,omitempty"`
	NewBlobBase64 string     `json:"new_blob_base64,omitempty"`
	Lines         []DiffLine `json:"lines"`
	Additions     int        `json:"additions"`
	Deletions     int        `json:"deletions"`
	Hunks         []Hunk     `json:"hunks"`
	// Uncommitted is true when the file has working-tree changes not yet
	// committed (staged or unstaged). Used by the blended branch-review diff to
	// mark files as committed vs uncommitted. Always false for commit/range diffs.
	Uncommitted bool `json:"uncommitted"`
}

// MarshalJSON emits a FileDiff whose slice fields are always arrays.
//
// A binary file never enters a hunk, so its Lines is never appended to and
// stays nil — and a nil slice marshals to `null`, not `[]`. Clients declare the
// field as a plain array and dereference it (`diff.lines.length`), so any
// commit touching a binary file took down the whole diff pane with "null is not
// an object". A JSON contract that promises an array has to send one even when
// there is nothing in it.
//
// Normalised here rather than at each producer so the guarantee holds for every
// endpoint that returns a FileDiff, including ones written later — the parser
// leaving Lines nil for a binary file is correct, it is only the wire format
// that must not expose it.
func (f FileDiff) MarshalJSON() ([]byte, error) {
	// A local type sheds FileDiff's method set, so json.Marshal below cannot
	// re-enter this function.
	type wire FileDiff
	out := wire(f)
	if out.Lines == nil {
		out.Lines = []DiffLine{}
	}
	if out.Hunks == nil {
		out.Hunks = []Hunk{}
	}
	return json.Marshal(out)
}

// MultiFileDiff is the diff for a commit (04 §3).
type MultiFileDiff struct {
	CommitHash        string     `json:"commitHash,omitempty"`
	CommitMessage     string     `json:"commitMessage,omitempty"`
	CommitDescription string     `json:"commitDescription,omitempty"`
	CommitAuthor      string     `json:"commitAuthor,omitempty"`
	CommitDate        *time.Time `json:"commitDate,omitempty"`
	Files             []FileDiff `json:"files"`
	TotalFiles        int        `json:"totalFiles"`
	TotalAdditions    int        `json:"totalAdditions"`
	TotalDeletions    int        `json:"totalDeletions"`
}

// Hunk is a contiguous changed block in a FileDiff. StartLine and EndLine are
// inclusive indices into FileDiff.Lines (04 §4).
type Hunk struct {
	HunkID    string `json:"hunkId"`
	Header    string `json:"header"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}
