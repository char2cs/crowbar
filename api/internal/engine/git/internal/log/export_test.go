package log

import gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"

func ExportedParseRecords(output string) ([]gitdomain.Commit, error) {
	return parseRecords(output)
}

func ExportedParseRecord(rec string) (gitdomain.Commit, error) {
	return parseRecord(rec)
}
