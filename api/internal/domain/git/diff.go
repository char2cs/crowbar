package git

import (
	"encoding/json"
	"time"
)

type DiffLine struct {
	LineType      DiffLineType `json:"line_type"`
	Content       string       `json:"content"`
	OldLineNumber *int         `json:"old_line_number,omitempty"`
	NewLineNumber *int         `json:"new_line_number,omitempty"`
	HunkID        string       `json:"hunkId,omitempty"`
}

type DiffLineType string

const (
	DiffLineAdded   DiffLineType = "added"
	DiffLineRemoved DiffLineType = "removed"
	DiffLineContext DiffLineType = "context"
	DiffLineHeader  DiffLineType = "header"
)

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
	Uncommitted   bool       `json:"uncommitted"`
}

func (f FileDiff) MarshalJSON() ([]byte, error) {
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

type Hunk struct {
	HunkID    string `json:"hunkId"`
	Header    string `json:"header"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}
