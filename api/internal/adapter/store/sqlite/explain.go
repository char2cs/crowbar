package sqlite

import (
	"errors"
	"io/fs"
	"syscall"
)

const (
	codeReadOnly = 8
	codeCorrupt  = 11
	codeFull     = 13
	codeCantOpen = 14
	codeNotADB   = 26
)

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

func refusedPath(err error) string {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) && pathErr.Path != "" {
		return " (" + pathErr.Path + ")"
	}
	return ""
}
