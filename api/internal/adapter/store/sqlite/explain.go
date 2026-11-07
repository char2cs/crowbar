package sqlite

import (
	"errors"
	"io/fs"
	"syscall"
)

// SQLite's primary result codes for the faults a user can act on.
const (
	codeReadOnly = 8
	codeCorrupt  = 11
	codeFull     = 13
	codeCantOpen = 14
	codeNotADB   = 26
)

// Explain names err in words a user can act on when it is a disk or database
// fault: read-only, full, or damaged. The driver's own text for some of them
// is misleading ("out of memory" for a file it cannot open).
func Explain(err error) (string, bool) {
	var coded interface{ Code() int }
	code := -1
	if errors.As(err, &coded) {
		code = coded.Code() & 0xff
	}
	switch {
	case errors.Is(err, syscall.ENOSPC) || code == codeFull:
		return "the disk holding Crowbar's data is full", true
	case errors.Is(err, syscall.EROFS) || errors.Is(err, fs.ErrPermission) || code == codeReadOnly || code == codeCantOpen:
		return "Crowbar's data folder is read-only, or Crowbar may not write to it" + refusedPath(err), true
	case code == codeCorrupt || code == codeNotADB:
		return "a database file in Crowbar's data folder is damaged", true
	default:
		return "", false
	}
}

// refusedPath names the file a refusal was about, when the error carries one.
func refusedPath(err error) string {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) && pathErr.Path != "" {
		return " (" + pathErr.Path + ")"
	}
	return ""
}
