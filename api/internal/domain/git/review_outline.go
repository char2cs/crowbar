package git

// HunkShape is one `@@` header of a diff reduced to its four numbers: where the
// hunk starts on each side and how many lines it spans there. The line counts
// include context lines, so they describe exactly how tall the hunk renders —
// which is the whole point of the shape, since the client reserves scroll space
// from it before any patch text is fetched.
//
// A count omitted from the header (`@@ -5 +5 @@`) means one line; a zero count
// (`@@ -1,2 +0,0 @@` for a deletion) means the side is absent.
type HunkShape struct {
	OldStart int `json:"oldStart"`
	OldLines int `json:"oldLines"`
	NewStart int `json:"newStart"`
	NewLines int `json:"newLines"`
}

// FileOutline is one file of a branch diff described by its hunk geometry alone
// — no diff content whatsoever. An outline is O(hunks), never O(lines), so the
// whole of a million-line diff can be described in about a megabyte and the
// client can lay out every file before deciding which patches to fetch.
//
// Renames carry the NEW path in Path (the path the patch endpoint is addressed
// with) and the source in OldPath. Binary files have no hunks at all: git emits
// no `@@` headers for them.
//
// IsPartial marks a file whose hunk count ran past the collection cap. Its Hunks
// hold the leading hunks only, so the geometry is a lower bound and the client
// must not treat the last hunk as the end of the file.
type FileOutline struct {
	Path      string      `json:"path"`
	OldPath   string      `json:"oldPath,omitempty"`
	Hunks     []HunkShape `json:"hunks"`
	IsPartial bool        `json:"isPartial"`
	IsBinary  bool        `json:"isBinary"`
}
