package status

import gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"

func ExportedParseAheadBehind(ab string) (int, int) {
	return parseAheadBehind(ab)
}

func ExportedParseOrdinaryEntries(line string) []gitdomain.GitFile {
	return parseOrdinaryEntries(line)
}

func ExportedParseRenamedEntry(line string) gitdomain.GitFile {
	return parseRenamedEntry(line)
}

func ExportedParseUnmergedEntry(line string) gitdomain.GitFile {
	return parseUnmergedEntry(line)
}

func ExportedIsConflict(xy string) bool {
	return isConflict(xy)
}

func ExportedCharToStatus(c rune) gitdomain.GitFileStatus {
	return charToStatus(c)
}
