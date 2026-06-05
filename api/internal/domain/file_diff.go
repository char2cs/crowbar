package domain

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
}
