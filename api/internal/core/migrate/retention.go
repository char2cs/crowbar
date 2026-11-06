package migrate

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// restoreNoteName is the file in migrate/backups that says how to restore.
const restoreNoteName = "README.txt"

const restoreNote = `Each file here is the Crowbar home as it stood before a migration rewrote it:
state/, LAYOUT and migrate/ledger.jsonl, and the files outside state/ the
migration read. Crowbar keeps the newest one and deletes the older ones once a
later migration completed.

To roll back to one: quit Crowbar, then

    cd <the Crowbar home>
    rm -rf state
    tar -xzf migrate/backups/<run-id>.tar.gz

The next start of a Crowbar that knows the migration runs it again; an older
Crowbar finds the home as it was before. An older Crowbar started on a home
that was not restored finds its own databases gone and shows chats without
their history.

A directory instead of a .tar.gz is a backup from an earlier build: copy its
state/, LAYOUT and ledger.jsonl back to state/, LAYOUT and migrate/ledger.jsonl.
`

// writeRestoreNote leaves the restore instructions beside the backups.
func writeRestoreNote(
	h home.Home,
) error {
	path := filepath.Join(h.MigrateDir(), backupsDirName, restoreNoteName)
	if err := os.WriteFile(path, []byte(restoreNote), 0o600); err != nil {
		return fmt.Errorf("migrate: write %s: %w", path, err)
	}
	return nil
}

// restoreHint is the command that restores the backup at path, for the log.
func restoreHint(
	h home.Home,
	path string,
) string {
	return fmt.Sprintf("quit Crowbar; cd %s && rm -rf state && tar -xzf %s", h.Root(), homeRel(h, path))
}

// listBackups is every whole backup on disk, oldest first: archives and the
// directories earlier builds wrote, named by the run that took them.
func listBackups(
	h home.Home,
) ([]string, error) {
	dir := filepath.Join(h.MigrateDir(), backupsDirName)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("migrate: backup: list %s: %w", dir, err)
	}
	var out []string
	for _, entry := range entries {
		run := strings.TrimSuffix(entry.Name(), backupExt)
		if _, err := time.Parse(runIDLayout, run); err != nil || entry.IsDir() == (run != entry.Name()) {
			continue
		}
		out = append(out, entry.Name())
	}
	// A run id is its start time in a fixed-width layout, so names sort by age.
	slices.SortFunc(out, func(a, b string) int {
		return strings.Compare(strings.TrimSuffix(a, backupExt), strings.TrimSuffix(b, backupExt))
	})
	for i, name := range out {
		out[i] = filepath.Join(dir, name)
	}
	return out, nil
}

// pruneBackups deletes every backup but the newest, once the ledger's latest
// run completed: the newest is then the image taken before the last migration
// that rewrote state/, and the older ones restore homes no build migrates from.
func pruneBackups(
	h home.Home,
	v ledgerView,
	log *slog.Logger,
) error {
	last, ok := v.lastRun()
	if !ok || last.Outcome != outcomeCompleted {
		return nil
	}
	backups, err := listBackups(h)
	if err != nil || len(backups) < 2 {
		return err
	}
	var failures []error
	for _, path := range backups[:len(backups)-1] {
		size := treeSize(path)
		if err := removeTree(path); err != nil {
			failures = append(failures, err)
			continue
		}
		log.Info("migrate: deleted a backup a newer one superseded", "backup", path, "bytes", size,
			"kept", backups[len(backups)-1])
	}
	return errors.Join(failures...)
}

// treeSize is the bytes under path, for the log; it is 0 if it cannot be read.
func treeSize(
	path string,
) int64 {
	stats, err := scanTree(path)
	if err != nil {
		return 0
	}
	return stats.bytes
}

// History is what the ledger says about the runs before this one, for a step
// that decides by them.
type History struct {
	home home.Home
	view ledgerView
}

// CompletedAt reports whether a run already ended completed with the home at
// layout or later: the home booted at that layout and its post-open steps ran.
func (h History) CompletedAt(
	layout int,
) bool {
	return slices.ContainsFunc(h.view.records, func(r record) bool {
		return r.Kind == kindRunEnd && r.Outcome == outcomeCompleted && layoutOf(r.To) >= layout
	})
}

// Notes is the latest note step id recorded under each key.
func (h History) Notes(
	id ID,
) map[string]Note {
	return h.view.notes(id.Number, id.Name)
}

// Marks are the keys step id checkpointed, across its runs.
func (h History) Marks(
	id ID,
) Marks {
	return h.view.checkpoints(id.Number, id.Name)
}

// Backup is the newest backup on disk, the one SameInBackup checks against;
// "" when there is none.
func (h History) Backup() (string, error) {
	backups, err := listBackups(h.home)
	if err != nil || len(backups) == 0 {
		return "", err
	}
	return backups[len(backups)-1], nil
}
