// export_test.go exposes internal functions for white-box unit tests.
package log

import "github.com/char2cs/crowbar/api/internal/domain"

// ExportedParseRecords wraps parseRecords.
func ExportedParseRecords(output string) ([]domain.Commit, error) {
	return parseRecords(output)
}

// ExportedParseRecord wraps parseRecord.
func ExportedParseRecord(rec string) (domain.Commit, error) {
	return parseRecord(rec)
}
