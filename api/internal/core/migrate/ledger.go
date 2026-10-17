package migrate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// ErrLedgerCorrupt is returned when a ledger line other than a torn final
// write cannot be parsed. The ledger is never repaired past a torn tail.
var ErrLedgerCorrupt = errors.New("migrate: ledger is corrupt")

const (
	ledgerName    = "ledger.jsonl"
	recordVersion = 1

	kindRunStart   = "run.start"
	kindStepStart  = "step.start"
	kindCheckpoint = "checkpoint"
	kindStepDone   = "step.done"
	kindStepFail   = "step.fail"
	kindRunEnd     = "run.end"

	outcomeApplied   = "applied"
	outcomeCompleted = "completed"
	outcomeFailed    = "failed"
)

// record is one ledger line. The JSON field names are the on-disk format.
type record struct {
	V       int       `json:"v"`
	Kind    string    `json:"kind"` // run.start|backup|step.start|checkpoint|step.done|step.fail|run.end
	At      time.Time `json:"at"`
	Run     string    `json:"run"`
	Version string    `json:"version,omitempty"`     // run.start: binary version
	From    *int      `json:"layout_from,omitempty"` // run.start
	To      *int      `json:"layout_to,omitempty"`   // run.end
	Pending []int     `json:"pending,omitempty"`     // run.start
	Step    int       `json:"step,omitempty"`
	Name    string    `json:"name,omitempty"`
	Phase   string    `json:"phase,omitempty"`
	Key     string    `json:"key,omitempty"` // checkpoint
	Counts  Counts    `json:"counts,omitempty"`
	Outcome string    `json:"outcome,omitempty"` // step.done: applied; run.end: completed|failed|interrupted
	Error   string    `json:"error,omitempty"`
	Path    string    `json:"path,omitempty"`  // backup
	Bytes   int64     `json:"bytes,omitempty"` // backup
}

// RunSummary is what the ledger says about one run. End and Outcome are zero
// while the run has no run.end; Counts sum the run's step.done records.
type RunSummary struct {
	ID, Version, Outcome, Error string
	Start, End                  time.Time
	LayoutFrom, LayoutTo        int
	Counts                      Counts
}

// ledgerView is the records of a ledger in append order.
type ledgerView struct {
	records []record
}

// applied reports whether a step.done with outcome applied exists for the
// step number and name.
func (v ledgerView) applied(
	n int,
	name string,
) bool {
	return slices.ContainsFunc(v.records, func(r record) bool {
		return r.Kind == kindStepDone && r.Outcome == outcomeApplied && r.Step == n && r.Name == name
	})
}

// checkpoints sums every checkpoint record of step n, across runs, by key.
func (v ledgerView) checkpoints(
	n int,
) map[string]Counts {
	out := map[string]Counts{}
	for _, r := range v.records {
		if r.Kind != kindCheckpoint || r.Step != n {
			continue
		}
		if out[r.Key] == nil {
			out[r.Key] = Counts{}
		}
		out[r.Key].Add(r.Counts)
	}
	return out
}

// danglingRun returns the id of the last run.start when it has no run.end.
func (v ledgerView) danglingRun() (string, bool) {
	start, rest, ok := v.latestRun()
	if !ok || slices.ContainsFunc(rest, endsRun(start.Run)) {
		return "", false
	}
	return start.Run, true
}

// lastRun summarises the last run.start and the records that belong to it.
func (v ledgerView) lastRun() (RunSummary, bool) {
	start, rest, ok := v.latestRun()
	if !ok {
		return RunSummary{}, false
	}
	sum := RunSummary{
		ID:         start.Run,
		Version:    start.Version,
		Start:      start.At,
		LayoutFrom: layoutOf(start.From),
		Counts:     Counts{},
	}
	for _, r := range rest {
		if r.Run == start.Run {
			sum.fold(r)
		}
	}
	return sum, true
}

func (v ledgerView) latestRun() (record, []record, bool) {
	for i := len(v.records) - 1; i >= 0; i-- {
		if v.records[i].Kind == kindRunStart {
			return v.records[i], v.records[i+1:], true
		}
	}
	return record{}, nil, false
}

func (s *RunSummary) fold(
	r record,
) {
	switch r.Kind {
	case kindStepDone:
		s.Counts.Add(r.Counts)
	case kindRunEnd:
		s.End, s.Outcome, s.Error, s.LayoutTo = r.At, r.Outcome, r.Error, layoutOf(r.To)
	}
}

func endsRun(
	id string,
) func(record) bool {
	return func(r record) bool {
		return r.Kind == kindRunEnd && r.Run == id
	}
}

func layoutOf(
	p *int,
) int {
	if p == nil {
		return 0
	}
	return *p
}

// ledgerImage is a parsed ledger file. intact is the byte length through the
// last complete good line; size > intact means the file ends in a torn write.
type ledgerImage struct {
	view   ledgerView
	size   int64
	intact int64
	absent bool
}

// loadLedger parses the file without touching it. A record exists only once its
// newline is on disk, so an unterminated tail was never acknowledged to its
// writer and is dropped, whether or not it parses. Any other bad line is corrupt.
func loadLedger(
	path string,
) (ledgerImage, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: path is the home's own ledger, not user input
	if errors.Is(err, fs.ErrNotExist) {
		return ledgerImage{absent: true}, nil
	}
	if err != nil {
		return ledgerImage{}, fmt.Errorf("migrate: ledger: read %s: %w", path, err)
	}
	img := ledgerImage{size: int64(len(raw))}
	rest := raw
	for lineNo := 1; len(rest) > 0; lineNo++ {
		text, after, terminated := bytes.Cut(rest, []byte{'\n'})
		if !terminated {
			break
		}
		r, err := parseRecord(text)
		if err != nil {
			return ledgerImage{}, fmt.Errorf("%w: %s: line %d: %w", ErrLedgerCorrupt, path, lineNo, err)
		}
		img.view.records = append(img.view.records, r)
		img.intact += int64(len(text)) + 1
		rest = after
	}
	return img, nil
}

func parseRecord(
	text []byte,
) (record, error) {
	var r record
	if err := json.Unmarshal(text, &r); err != nil {
		return record{}, err
	}
	if r.Kind == "" {
		return record{}, errors.New("record has no kind")
	}
	return r, nil
}

// readLedger reads the ledger at path without creating or repairing anything;
// an absent file is an empty ledger.
func readLedger(
	path string,
) (ledgerView, error) {
	img, err := loadLedger(path)
	return img.view, err
}

// ledger is the open, append-only run ledger. It is not safe for concurrent use.
// A failed write may leave a fragment of a line, so broken keeps every later
// append from fusing onto it; the next openLedger cuts the fragment off.
type ledger struct {
	view   ledgerView
	file   *os.File
	broken error
}

// openLedger reads the ledger at path, cuts a torn final write off the file and
// opens it for appending, creating the file and its directory when absent.
func openLedger(
	path string,
) (*ledger, error) {
	img, err := loadLedger(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("migrate: ledger: create %s: %w", dir, err)
	}
	if img.size > img.intact {
		if err := os.Truncate(path, img.intact); err != nil {
			return nil, fmt.Errorf("migrate: ledger: drop torn tail of %s: %w", path, err)
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600) //nolint:gosec // G304: path is the home's own ledger, not user input
	if err != nil {
		return nil, fmt.Errorf("migrate: ledger: open %s: %w", path, err)
	}
	if img.absent {
		if err := syncDir(dir); err != nil {
			return nil, errors.Join(err, file.Close())
		}
	}
	return &ledger{view: img.view, file: file}, nil
}

// append stamps r with the ledger format version and durably adds it as one
// line. The view keeps its own copy, and only learns of a record once it is
// synced. After a failed write or sync every append returns that failure.
func (l *ledger) append(
	r record,
) error {
	if l.broken != nil {
		return l.broken
	}
	if r.Kind == "" {
		return errors.New("migrate: ledger: append: record has no kind")
	}
	r = r.detached()
	r.V = recordVersion
	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("migrate: ledger: encode %s record: %w", r.Kind, err)
	}
	if _, err := l.file.Write(append(line, '\n')); err != nil {
		return l.poison("append", r.Kind, err)
	}
	if err := l.file.Sync(); err != nil {
		return l.poison("sync", r.Kind, err)
	}
	l.view.records = append(l.view.records, r)
	return nil
}

func (l *ledger) poison(
	op string,
	kind string,
	err error,
) error {
	l.broken = fmt.Errorf("migrate: ledger: %s %s record: %w", op, kind, err)
	return l.broken
}

// detached returns r sharing no maps, slices or pointers with the caller.
func (r record) detached() record {
	r.Counts = maps.Clone(r.Counts)
	r.Pending = slices.Clone(r.Pending)
	r.From, r.To = copyInt(r.From), copyInt(r.To)
	return r
}

func copyInt(
	p *int,
) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// Close closes the ledger file. It is safe to call twice.
func (l *ledger) Close() error {
	if err := l.file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		return fmt.Errorf("migrate: ledger: close: %w", err)
	}
	return nil
}
