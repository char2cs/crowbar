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

var errLedgerCorrupt = errors.New("migrate: ledger is corrupt")

const (
	ledgerName    = "ledger.jsonl"
	recordVersion = 1

	kindRunStart   = "run.start"
	kindBackup     = "backup"
	kindStepStart  = "step.start"
	kindCheckpoint = "checkpoint"
	kindNote       = "note"
	kindStepDone   = "step.done"
	kindStepFail   = "step.fail"
	kindRunEnd     = "run.end"

	outcomeApplied     = "applied"
	outcomeCompleted   = "completed"
	outcomeFailed      = "failed"
	outcomeInterrupted = "interrupted"
)

type record struct {
	V       int       `json:"v"`
	Kind    string    `json:"kind"`
	At      time.Time `json:"at"`
	Run     string    `json:"run"`
	Version string    `json:"version,omitempty"`
	From    *int      `json:"layout_from,omitempty"`
	To      *int      `json:"layout_to,omitempty"`
	Pending []int     `json:"pending,omitempty"`
	Step    int       `json:"step,omitempty"`
	Name    string    `json:"name,omitempty"`
	Phase   string    `json:"phase,omitempty"`
	Key     string    `json:"key,omitempty"`
	Detail  string    `json:"detail,omitempty"`
	Counts  Counts    `json:"counts,omitempty"`
	Outcome string    `json:"outcome,omitempty"`
	Error   string    `json:"error,omitempty"`
	Path    string    `json:"path,omitempty"`
	Bytes   int64     `json:"bytes,omitempty"`
}

type RunSummary struct {
	ID, Version, Outcome, Error string
	Start, End                  time.Time
	LayoutFrom, LayoutTo        int
	Counts                      Counts
}

type ledgerView struct {
	records []record
}

func (v ledgerView) applied(
	n int,
	name string,
) bool {
	return slices.ContainsFunc(v.records, func(r record) bool {
		return r.Kind == kindStepDone && r.Outcome == outcomeApplied && r.Step == n && r.Name == name
	})
}

func (v ledgerView) doneIn(
	run string,
	n int,
) bool {
	return slices.ContainsFunc(v.records, func(r record) bool {
		return r.Kind == kindStepDone && r.Outcome == outcomeApplied && r.Run == run && r.Step == n
	})
}

func (v ledgerView) checkpoints(
	n int,
	name string,
) Marks {
	out := Marks{}
	for _, r := range v.records {
		if r.Kind != kindCheckpoint || r.Step != n || r.Name != name {
			continue
		}
		if out[r.Key] == nil {
			out[r.Key] = Counts{}
		}
		out[r.Key].Add(r.Counts)
	}
	return out
}

func (v ledgerView) notes(
	n int,
	name string,
) map[string]Note {
	out := map[string]Note{}
	for _, r := range v.records {
		if r.Kind == kindNote && r.Step == n && r.Name == name {
			out[r.Key] = Note{Detail: r.Detail, Counts: maps.Clone(r.Counts)}
		}
	}
	return out
}

func (v ledgerView) settled(
	n int,
	name string,
) map[string]bool {
	out, since := map[string]bool{}, map[string]bool{}
	for _, r := range v.records {
		if r.Step != n || r.Name != name {
			continue
		}
		switch {
		case r.Kind == kindCheckpoint:
			since[r.Key] = true
		case r.Kind == kindStepDone && r.Outcome == outcomeApplied:
			maps.Copy(out, since)
		}
	}
	return out
}

func (v ledgerView) ended(
	r record,
) ledgerView {
	return ledgerView{records: append(slices.Clone(v.records), r)}
}

func (v ledgerView) danglingRun() (record, bool) {
	start, rest, ok := v.latestRun()
	if !ok || slices.ContainsFunc(rest, endsRun(start.Run)) {
		return record{}, false
	}
	return start, true
}

func (v ledgerView) backupInForce() record {
	for i := len(v.records) - 1; i >= 0; i-- {
		r := v.records[i]
		switch {
		case r.Kind == kindRunEnd && r.Outcome == outcomeCompleted:
			return record{}
		case r.Kind == kindBackup:
			return r
		}
	}
	return record{}
}

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

type ledgerImage struct {
	view   ledgerView
	size   int64
	intact int64
	absent bool
}

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
			return ledgerImage{}, fmt.Errorf("%w: %s: line %d: %w", errLedgerCorrupt, path, lineNo, err)
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

func readLedger(
	path string,
) (ledgerView, error) {
	img, err := loadLedger(path)
	return img.view, err
}

type ledger struct {
	view   ledgerView
	file   *os.File
	broken error
}

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

func (l *ledger) Close() error {
	if err := l.file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		return fmt.Errorf("migrate: ledger: close: %w", err)
	}
	return nil
}
