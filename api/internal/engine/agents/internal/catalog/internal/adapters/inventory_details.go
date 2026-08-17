package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog/internal/normalize"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// inventoryDetails reads a JSON inventory and then asks the provider about each
// enabled row, in parallel and under a declared concurrency cap.
type inventoryDetails struct{}

type row struct {
	id     string
	source string
}

type detailResult struct {
	candidates []Candidate
	warning    string
	err        error
}

func (a inventoryDetails) Probe(
	ctx context.Context,
	s *spec.SlashCatalogSpec,
	runner models.Runner,
) (Result, error) {
	p := &s.Pipeline
	raw, err := runner.Run(ctx, p.Command)
	if err != nil {
		return Result{}, err
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return Result{}, ErrMalformedOutput
	}

	values := selectPath([]any{document}, p.RowsPath)
	rows, warnings, err := parseRows(values, p)
	if err != nil {
		return Result{}, err
	}
	// A valid inventory may legitimately contain no enabled rows. That is an empty
	// menu, not a broken one.
	if len(rows) == 0 {
		return Result{Candidates: []Candidate{}, Warnings: warnings}, nil
	}
	if max := s.EffectiveMaxItems(); len(rows) > max {
		rows = rows[:max]
		warnings = append(warnings,
			"Provider inventory exceeded the safe expansion limit; some sources were omitted.")
	}

	results := a.fanOut(ctx, p, rows, s.EffectiveMaxItems(), runner)
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	candidates := make([]Candidate, 0, len(results))
	for _, result := range results {
		if result.err != nil {
			return Result{}, result.err
		}
		candidates = append(candidates, result.candidates...)
		if result.warning != "" {
			warnings = append(warnings, result.warning)
		}
	}
	return Result{Candidates: candidates, Warnings: warnings}, nil
}

// parseRows keeps only enabled rows with a usable id.
//
// An id becomes one argv element of the detail command, so it is bounded and
// screened: control characters would corrupt the command line, and a leading dash
// would turn the value into a flag.
func parseRows(values []any, p *spec.CatalogPipelineSpec) ([]row, []string, error) {
	rows := make([]row, 0, len(values))
	objectRows, invalidRows := 0, 0
	for _, value := range values {
		object, ok := value.(map[string]any)
		if !ok {
			invalidRows++
			continue
		}
		objectRows++
		if enabled, _ := lookupField(object, p.EnabledField).(bool); !enabled {
			continue
		}
		rawID, _ := lookupField(object, p.IDField).(string)
		id := strings.TrimSpace(rawID)
		if id == "" || id != normalize.StripComposerControls(id) || strings.HasPrefix(id, "-") {
			invalidRows++
			continue
		}
		id = normalize.TruncateBytes(normalize.StripControls(id), normalize.MaxIDBytes)
		rows = append(rows, row{id: id, source: sourceOf(id, p.SourcePattern)})
	}
	// Output that had rows but none of them objects is not an empty inventory —
	// it is output the descriptor cannot read.
	if len(values) > 0 && objectRows == 0 {
		return nil, nil, ErrMalformedOutput
	}
	var warnings []string
	if invalidRows > 0 {
		warnings = append(warnings, "Some malformed provider inventory rows were omitted.")
	}
	return rows, warnings, nil
}

func (inventoryDetails) fanOut(
	ctx context.Context,
	p *spec.CatalogPipelineSpec,
	rows []row,
	maxItems int,
	runner models.Runner,
) []detailResult {
	results := make([]detailResult, len(rows))
	slots := make(chan struct{}, p.EffectiveDetailConcurrency())
	var wg sync.WaitGroup
	for i := range rows {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				return
			}
			results[i] = detail(ctx, p, rows[i], maxItems, runner)
		}(i)
	}
	wg.Wait()
	return results
}

// detail asks the provider about one inventory row.
//
// A row that simply fails degrades to a warning rather than failing the whole
// catalogue: one uninspectable plugin should cost that plugin's entries, not the
// menu. Failures that indicate the WHOLE probe is compromised — the output
// ceiling, a missing executable, cancellation — do propagate, because continuing
// past them would publish a silently partial answer as a complete one.
func detail(
	ctx context.Context,
	p *spec.CatalogPipelineSpec,
	r row,
	maxItems int,
	runner models.Runner,
) detailResult {
	argv := make([]string, len(p.DetailCommand))
	for i, arg := range p.DetailCommand {
		argv[i] = strings.ReplaceAll(arg, "{id}", r.id)
	}
	raw, err := runner.Run(ctx, argv)
	if err != nil {
		if fatal(err) {
			return detailResult{err: err}
		}
		return detailResult{warning: warn(r.source, "could not be inspected")}
	}

	pattern, _ := regexp.Compile(p.DetailPattern)
	match := pattern.FindSubmatch(raw)
	if match == nil {
		if emptyInventory(p.DetailEmptyPattern, raw) {
			return detailResult{}
		}
		return detailResult{warning: warn(r.source, "returned an unrecognized component inventory")}
	}
	list := namedCapturesBytes(pattern, match)[p.DetailItemsGroup]
	return detailResult{candidates: splitItems(list, p.DetailSeparator, r, maxItems)}
}

func fatal(err error) bool {
	return errors.Is(err, exec.ErrOutputLimit) ||
		errors.Is(err, exec.ErrCommandUnavailable) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// emptyInventory distinguishes "this source has no components" from "this output
// is unreadable". Without it the empty case is reported as a parse failure, and a
// plugin that legitimately ships zero skills looks broken.
func emptyInventory(pattern string, raw []byte) bool {
	if pattern == "" {
		return false
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.Match(raw)
}

func splitItems(list, separator string, r row, maxItems int) []Candidate {
	names := strings.SplitN(list, separator, maxItems+1)
	out := make([]Candidate, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, Candidate{Name: name, Source: r.source, ID: r.id})
	}
	return out
}

func sourceOf(id, pattern string) string {
	if pattern == "" {
		return normalize.Source(id)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	match := re.FindStringSubmatch(id)
	if match == nil {
		return ""
	}
	return normalize.Source(namedCaptures(re, match)["source"])
}

func warn(source, reason string) string {
	if source == "" {
		return "One catalog source " + reason + "."
	}
	return "Catalog source " + source + " " + reason + "."
}
