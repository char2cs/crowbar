// export_test.go exposes internal functions for white-box unit tests.
package status

import gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"

// ExportedParseAheadBehind wraps parseAheadBehind.
func ExportedParseAheadBehind(ab string) (int, int) {
	return parseAheadBehind(ab)
}

// ExportedParseOrdinaryEntries wraps parseOrdinaryEntries.
func ExportedParseOrdinaryEntries(line string) []gitdomain.GitFile {
	return parseOrdinaryEntries(line)
}

// ExportedParseRenamedEntry wraps parseRenamedEntry.
func ExportedParseRenamedEntry(line string) gitdomain.GitFile {
	return parseRenamedEntry(line)
}

// ExportedParseUnmergedEntry wraps parseUnmergedEntry.
func ExportedParseUnmergedEntry(line string) gitdomain.GitFile {
	return parseUnmergedEntry(line)
}

// ExportedIsConflict wraps isConflict.
func ExportedIsConflict(xy string) bool {
	return isConflict(xy)
}

// ExportedCharToStatus wraps charToStatus.
func ExportedCharToStatus(c rune) gitdomain.GitFileStatus {
	return charToStatus(c)
}
