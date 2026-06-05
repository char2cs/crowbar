// export_test.go exposes internal functions for white-box unit tests.
package log

import gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"

// ExportedParseRecords wraps parseRecords.
func ExportedParseRecords(output string) ([]gitdomain.Commit, error) {
	return parseRecords(output)
}

// ExportedParseRecord wraps parseRecord.
func ExportedParseRecord(rec string) (gitdomain.Commit, error) {
	return parseRecord(rec)
}
