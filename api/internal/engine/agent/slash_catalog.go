package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
)

var (
	ErrSlashCatalogUnsupported        = errors.New("agent: provider does not support slash catalog discovery")
	ErrSlashCatalogInvalidWorkdir     = errors.New("agent: slash catalog worktree is invalid")
	ErrSlashCatalogTimeout            = errors.New("agent: slash catalog probe timed out")
	ErrSlashCatalogOutputLimit        = errors.New("agent: slash catalog probe exceeded its output limit")
	ErrSlashCatalogCommandUnavailable = errors.New("agent: slash catalog provider command is unavailable")
	ErrSlashCatalogCommandFailed      = errors.New("agent: slash catalog provider command failed")
	ErrSlashCatalogMalformedOutput    = errors.New("agent: slash catalog provider output is malformed")
)

// SlashCatalog is an ephemeral, provider-neutral capability result. It contains
// no raw command output, filesystem locators, or provider config values.
type SlashCatalog struct {
	ProviderID   string
	Completeness CatalogCompleteness
	Items        []SlashCatalogItem
	Warnings     []string
}

type SlashCatalogItem struct {
	ID          string
	Kind        string
	Label       string
	Description string
	InsertText  string
	Source      string
}

// SlashCatalogProbeOptions supplies only execution context. A nil Env means the
// daemon's current environment; descriptor spawn.env.clear entries are removed
// before every command. Cwd must be the absolute chat worktree.
type SlashCatalogProbeOptions struct {
	Cwd string
	Env []string
	// AcquireProcess, when non-nil, acquires one caller-owned process permit.
	// ProbeSlashCatalog invokes it immediately before every provider command and
	// releases the permit after Wait returns. This keeps the engine independent of
	// daemon policy while allowing all concurrent chat probes to share one budget.
	AcquireProcess func(context.Context) (release func(), err error)
}

// ProbeSlashCatalog executes the descriptor-declared deterministic pipeline.
// It starts no shell, reads no provider-owned files itself, retains no cache, and
// discards stdout/stderr after mapping the normalized result.
func ProbeSlashCatalog(ctx context.Context, d *Descriptor, opts SlashCatalogProbeOptions) (SlashCatalog, error) {
	if d == nil || d.Presentation.SlashCatalog == nil {
		return SlashCatalog{}, ErrSlashCatalogUnsupported
	}
	if opts.Cwd == "" || !filepath.IsAbs(opts.Cwd) {
		return SlashCatalog{}, ErrSlashCatalogInvalidWorkdir
	}
	info, err := os.Stat(opts.Cwd)
	if err != nil || !info.IsDir() {
		return SlashCatalog{}, ErrSlashCatalogInvalidWorkdir
	}
	if err := d.validateSlashCatalog(); err != nil {
		return SlashCatalog{}, err
	}

	spec := d.Presentation.SlashCatalog
	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(spec.effectiveTimeoutMS())*time.Millisecond)
	defer cancel()

	baseEnv := opts.Env
	if baseEnv == nil {
		baseEnv = os.Environ()
	}
	cleanCwd := filepath.Clean(opts.Cwd)
	probeEnv := clearEnv(baseEnv, d.Spawn.Env.Clear)
	probeEnv = replaceEnvValue(probeEnv, "PWD", cleanCwd)
	exec := &boundedCatalogExecutor{
		executable:     resolveCatalogExecutable(d.Spawn.Cmd, baseEnv),
		cwd:            cleanCwd,
		env:            probeEnv,
		stdoutBudget:   newOutputBudget(spec.effectiveMaxStdoutBytes()),
		stderrBudget:   newOutputBudget(spec.effectiveMaxStderrBytes()),
		acquireProcess: opts.AcquireProcess,
	}
	result, err := probeSlashCatalog(probeCtx, d, exec)
	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return SlashCatalog{}, ErrSlashCatalogTimeout
		}
		if ctx.Err() != nil {
			return SlashCatalog{}, ctx.Err()
		}
		return SlashCatalog{}, err
	}
	return result, nil
}

func replaceEnvValue(env []string, name, value string) []string {
	prefix := name + "="
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			out = append(out, entry)
		}
	}
	return append(out, prefix+value)
}

type catalogExecutor interface {
	Run(context.Context, []string) ([]byte, error)
}

type boundedCatalogExecutor struct {
	executable     string
	cwd            string
	env            []string
	stdoutBudget   *outputBudget
	stderrBudget   *outputBudget
	acquireProcess func(context.Context) (release func(), err error)
}

func (e *boundedCatalogExecutor) Run(ctx context.Context, argv []string) ([]byte, error) {
	if e.acquireProcess != nil {
		release, err := e.acquireProcess(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
	}

	commandCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(commandCtx, e.executable, argv...)
	cmd.Dir = e.cwd
	cmd.Env = append([]string(nil), e.env...)
	// Cancellation and timeout must reach the provider's OWN helpers, not just the
	// process Crowbar forked. A provider CLI that shells out leaves those children
	// running under the default Cancel, which signals one pid; isolating the group
	// and signalling it negatively kills the tree the probe created.
	isolateProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcessTree(cmd.Process) }
	// If the provider command exits but a descendant inherited its pipes, do not
	// let that descendant strand an abandoned HTTP request indefinitely.
	cmd.WaitDelay = 500 * time.Millisecond

	stdout := newBoundedBuffer(e.stdoutBudget, cancel)
	stderr := newBoundedBuffer(e.stderrBudget, cancel)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()

	if stdout.Exceeded() || stderr.Exceeded() {
		return nil, ErrSlashCatalogOutputLimit
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return nil, ErrSlashCatalogCommandUnavailable
		}
		// Intentionally omit executable, argv, cwd, stdout, and stderr. A provider
		// may print credentials or config locations on failure.
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, &slashCatalogCommandError{exitCode: exitErr.ExitCode()}
		}
		return nil, ErrSlashCatalogCommandFailed
	}
	return stdout.Bytes(), nil
}

type slashCatalogCommandError struct {
	exitCode int
}

func (e *slashCatalogCommandError) Error() string {
	return fmt.Sprintf("%s (exit %d)", ErrSlashCatalogCommandFailed, e.exitCode)
}

func (e *slashCatalogCommandError) Unwrap() error { return ErrSlashCatalogCommandFailed }

type boundedBuffer struct {
	buf      bytes.Buffer
	budget   *outputBudget
	exceeded bool
	cancel   context.CancelFunc
}

type outputBudget struct {
	mu        sync.Mutex
	remaining int
}

func newOutputBudget(max int) *outputBudget {
	return &outputBudget{remaining: max}
}

func (b *outputBudget) consume(requested int) (accepted int, exceeded bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if requested <= b.remaining {
		b.remaining -= requested
		return requested, false
	}
	accepted = b.remaining
	b.remaining = 0
	return accepted, true
}

func newBoundedBuffer(budget *outputBudget, cancel context.CancelFunc) *boundedBuffer {
	return &boundedBuffer{budget: budget, cancel: cancel}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.exceeded {
		return 0, ErrSlashCatalogOutputLimit
	}
	accepted, exceeded := b.budget.consume(len(p))
	if accepted > 0 {
		_, _ = b.buf.Write(p[:accepted])
	}
	if exceeded {
		b.exceeded = true
		b.cancel()
		return accepted, ErrSlashCatalogOutputLimit
	}
	return accepted, nil
}

func (b *boundedBuffer) Bytes() []byte {
	return append([]byte(nil), b.buf.Bytes()...)
}

func (b *boundedBuffer) Exceeded() bool { return b.exceeded }

func probeSlashCatalog(ctx context.Context, d *Descriptor, runner catalogExecutor) (SlashCatalog, error) {
	spec := d.Presentation.SlashCatalog
	result := SlashCatalog{
		ProviderID:   d.ID,
		Completeness: spec.Completeness,
		Items:        []SlashCatalogItem{},
		Warnings:     completenessWarnings(spec.Completeness),
	}

	var (
		items    []catalogCandidate
		warnings []string
		err      error
	)
	switch spec.Pipeline.Adapter {
	case CatalogAdapterJSONTextSection:
		items, err = probeJSONTextSection(ctx, spec, runner)
	case CatalogAdapterJSONInventoryDetails:
		items, warnings, err = probeJSONInventoryDetails(ctx, spec, runner)
	default:
		return SlashCatalog{}, fmt.Errorf("%w: unsupported catalog adapter", ErrSlashCatalogMalformedOutput)
	}
	if err != nil {
		return SlashCatalog{}, err
	}
	result.Warnings = appendWarnings(result.Warnings, warnings...)
	result.Items, warnings = normalizeCatalogItems(items, spec.effectiveMaxItems())
	result.Warnings = appendWarnings(result.Warnings, warnings...)
	return result, nil
}

type catalogCandidate struct {
	name        string
	description string
	source      string
	id          string
	mapping     CatalogItemMapping
}

func probeJSONTextSection(ctx context.Context, spec *SlashCatalogSpec, runner catalogExecutor) ([]catalogCandidate, error) {
	pipeline := &spec.Pipeline
	raw, err := runner.Run(ctx, pipeline.Command)
	if err != nil {
		return nil, err
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, ErrSlashCatalogMalformedOutput
	}
	values := selectJSONPath([]any{document}, pipeline.TextPath)
	pattern, _ := regexp.Compile(pipeline.ItemPattern) // descriptor validation compiled it
	sectionFound := false
	items := make([]catalogCandidate, 0)
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		for _, section := range literalSections(text, pipeline.StartMarker, pipeline.EndMarker) {
			sectionFound = true
			remaining := spec.effectiveMaxItems() + 1 - len(items)
			if remaining <= 0 {
				break
			}
			for _, match := range pattern.FindAllStringSubmatch(section, remaining) {
				captures := namedCaptures(pattern, match)
				name := strings.TrimSpace(captures["name"])
				if name == "" {
					continue
				}
				items = append(items, catalogCandidate{
					name:        name,
					description: strings.TrimSpace(captures["description"]),
					source:      strings.TrimSpace(captures["source"]),
					mapping:     pipeline.Item,
				})
			}
		}
	}
	if !sectionFound {
		return nil, ErrSlashCatalogMalformedOutput
	}
	return items, nil
}

type inventoryRow struct {
	id     string
	source string
}

type inventoryDetailResult struct {
	items   []catalogCandidate
	warning string
	err     error
}

func probeJSONInventoryDetails(ctx context.Context, spec *SlashCatalogSpec, runner catalogExecutor) ([]catalogCandidate, []string, error) {
	pipeline := &spec.Pipeline
	raw, err := runner.Run(ctx, pipeline.Command)
	if err != nil {
		return nil, nil, err
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, nil, ErrSlashCatalogMalformedOutput
	}
	values := selectJSONPath([]any{document}, pipeline.RowsPath)
	warnings := []string{}
	rows := make([]inventoryRow, 0, len(values))
	objectRows := 0
	invalidRows := 0
	for _, value := range values {
		row, ok := value.(map[string]any)
		if !ok {
			invalidRows++
			continue
		}
		objectRows++
		enabled, _ := lookupJSONField(row, pipeline.EnabledField).(bool)
		if !enabled {
			continue
		}
		rawID, _ := lookupJSONField(row, pipeline.IDField).(string)
		id := strings.TrimSpace(rawID)
		if id == "" || id != stripComposerControls(id) || strings.HasPrefix(id, "-") {
			invalidRows++
			continue
		}
		// The ID is provider output used only as one argv replacement. Bound it so a
		// malformed inventory cannot manufacture an enormous command line.
		id = truncateUTF8Bytes(stripControls(id), 512)
		rows = append(rows, inventoryRow{id: id, source: inventorySource(id, pipeline.SourcePattern)})
	}
	if len(values) > 0 && objectRows == 0 {
		return nil, nil, ErrSlashCatalogMalformedOutput
	}
	if invalidRows > 0 {
		warnings = append(warnings, "Some malformed provider inventory rows were omitted.")
	}
	if len(values) > 0 && len(rows) == 0 {
		// A valid inventory may legitimately contain no enabled rows.
		return []catalogCandidate{}, warnings, nil
	}

	maxRows := spec.effectiveMaxItems()
	if len(rows) > maxRows {
		rows = rows[:maxRows]
		warnings = append(warnings, "Provider inventory exceeded the safe expansion limit; some sources were omitted.")
	}
	results := make([]inventoryDetailResult, len(rows))
	sem := make(chan struct{}, pipeline.effectiveDetailConcurrency())
	var wg sync.WaitGroup
	for i := range rows {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			results[i] = probeInventoryDetail(ctx, pipeline, rows[i], spec.effectiveMaxItems(), runner)
		}(i)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}

	items := make([]catalogCandidate, 0)
	for _, result := range results {
		if result.err != nil {
			return nil, nil, result.err
		}
		items = append(items, result.items...)
		if result.warning != "" {
			warnings = append(warnings, result.warning)
		}
	}
	return items, warnings, nil
}

func probeInventoryDetail(ctx context.Context, pipeline *CatalogPipelineSpec, row inventoryRow, maxItems int, runner catalogExecutor) inventoryDetailResult {
	argv := make([]string, len(pipeline.DetailCommand))
	for i, arg := range pipeline.DetailCommand {
		argv[i] = strings.ReplaceAll(arg, "{id}", row.id)
	}
	raw, err := runner.Run(ctx, argv)
	if err != nil {
		if errors.Is(err, ErrSlashCatalogOutputLimit) ||
			errors.Is(err, ErrSlashCatalogCommandUnavailable) ||
			errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return inventoryDetailResult{err: err}
		}
		return inventoryDetailResult{warning: detailWarning(row.source, "could not be inspected")}
	}
	pattern, _ := regexp.Compile(pipeline.DetailPattern)
	match := pattern.FindSubmatch(raw)
	if match == nil {
		if pipeline.DetailEmptyPattern != "" {
			emptyPattern, _ := regexp.Compile(pipeline.DetailEmptyPattern)
			if emptyPattern.Match(raw) {
				return inventoryDetailResult{}
			}
		}
		return inventoryDetailResult{warning: detailWarning(row.source, "returned an unrecognized component inventory")}
	}
	captures := namedCapturesBytes(pattern, match)
	list := captures[pipeline.DetailItemsGroup]
	names := strings.SplitN(list, pipeline.DetailSeparator, maxItems+1)
	items := make([]catalogCandidate, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		items = append(items, catalogCandidate{
			name:    name,
			source:  row.source,
			id:      row.id,
			mapping: pipeline.Item,
		})
	}
	return inventoryDetailResult{items: items}
}

func inventorySource(id, pattern string) string {
	if pattern == "" {
		return sanitizeCatalogSource(id)
	}
	re, _ := regexp.Compile(pattern)
	match := re.FindStringSubmatch(id)
	if match == nil {
		return ""
	}
	return sanitizeCatalogSource(namedCaptures(re, match)["source"])
}

func detailWarning(source, reason string) string {
	if source == "" {
		return "One catalog source " + reason + "."
	}
	return "Catalog source " + source + " " + reason + "."
}

func selectJSONPath(values []any, path string) []any {
	segments := strings.Split(path, ".")
	current := values
	for _, segment := range segments {
		if segment == "" {
			return nil
		}
		expandArray := strings.HasSuffix(segment, "[]")
		key := strings.TrimSuffix(segment, "[]")
		next := make([]any, 0)
		for _, value := range current {
			selected := value
			if key != "" {
				object, ok := value.(map[string]any)
				if !ok {
					continue
				}
				selected, ok = object[key]
				if !ok {
					continue
				}
			}
			if expandArray {
				array, ok := selected.([]any)
				if ok {
					next = append(next, array...)
				}
				continue
			}
			next = append(next, selected)
		}
		current = next
	}
	return current
}

func lookupJSONField(row map[string]any, path string) any {
	var current any = row
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = object[part]
		if !ok {
			return nil
		}
	}
	return current
}

func literalSections(text, start, end string) []string {
	sections := []string{}
	for {
		startAt := strings.Index(text, start)
		if startAt < 0 {
			return sections
		}
		text = text[startAt+len(start):]
		endAt := strings.Index(text, end)
		if endAt < 0 {
			return sections
		}
		sections = append(sections, text[:endAt])
		text = text[endAt+len(end):]
	}
}

func namedCaptures(re *regexp.Regexp, match []string) map[string]string {
	out := make(map[string]string, len(match))
	for i, name := range re.SubexpNames() {
		if i > 0 && name != "" && i < len(match) {
			out[name] = match[i]
		}
	}
	return out
}

func namedCapturesBytes(re *regexp.Regexp, match [][]byte) map[string]string {
	out := make(map[string]string, len(match))
	for i, name := range re.SubexpNames() {
		if i > 0 && name != "" && i < len(match) {
			out[name] = string(match[i])
		}
	}
	return out
}

func normalizeCatalogItems(candidates []catalogCandidate, maxItems int) ([]SlashCatalogItem, []string) {
	items := make([]SlashCatalogItem, 0, min(len(candidates), maxItems))
	seen := make(map[string]struct{}, len(candidates))
	truncated := false
	for _, candidate := range candidates {
		vars := map[string]string{
			"name":        redactCatalogText(candidate.name),
			"description": redactCatalogText(candidate.description),
			"source":      sanitizeCatalogSource(candidate.source),
			"id":          sanitizeCatalogSource(candidate.id),
		}
		label := truncateRunes(stripControls(expandCatalogMapping(candidate.mapping.Label, vars)), 256)
		description := truncateUTF8Bytes(stripControls(expandCatalogMapping(candidate.mapping.Description, vars)), 2<<10)
		description = redactCatalogText(description)
		insertText := truncateUTF8Bytes(stripComposerControls(expandCatalogMapping(candidate.mapping.InsertText, vars)), 512)
		source := truncateRunes(sanitizeCatalogSource(expandCatalogMapping(candidate.mapping.Source, vars)), 256)
		if label == "" || insertText == "" {
			continue
		}
		key := source + "\x00" + insertText
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if len(items) == maxItems {
			truncated = true
			continue
		}
		hash := sha256.Sum256([]byte(key))
		items = append(items, SlashCatalogItem{
			ID:          "skill-" + hex.EncodeToString(hash[:8]),
			Kind:        CatalogItemKindSkill,
			Label:       label,
			Description: description,
			InsertText:  insertText,
			Source:      source,
		})
	}
	if truncated {
		return items, []string{"Catalog results were truncated to the safe item limit."}
	}
	return items, nil
}

func expandCatalogMapping(template string, values map[string]string) string {
	return strings.NewReplacer(
		"{name}", values["name"],
		"{description}", values["description"],
		"{source}", values["source"],
		"{id}", values["id"],
	).Replace(template)
}

func completenessWarnings(completeness CatalogCompleteness) []string {
	switch completeness {
	case CatalogCompletenessModelVisible:
		return []string{"This catalog contains model-visible skills and may differ from the provider's native menu."}
	case CatalogCompletenessPluginOnly:
		return []string{"This catalog contains enabled plugin skills only; standalone skills may be available in the native terminal."}
	default:
		return []string{}
	}
}

func appendWarnings(existing []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(additions))
	for _, warning := range existing {
		seen[warning] = struct{}{}
	}
	for _, warning := range additions {
		warning = truncateUTF8Bytes(stripComposerControls(redactCatalogText(warning)), 512)
		if warning == "" {
			continue
		}
		if _, exists := seen[warning]; exists {
			continue
		}
		if len(existing) == 64 {
			break
		}
		seen[warning] = struct{}{}
		existing = append(existing, warning)
	}
	return existing
}

var (
	unixPathPattern    = regexp.MustCompile(`(^|[\s(])(/(?:[^/\s),;]+/)+[^\s),;]+)`)
	homePathPattern    = regexp.MustCompile(`(^|[\s(])(~/(?:[^/\s),;]+/)*[^\s),;]+)`)
	windowsPathPattern = regexp.MustCompile(`(?i)(^|[\s(])([a-z]:\\(?:[^\\\s),;]+\\)+[^\s),;]+)`)
	bearerPattern      = regexp.MustCompile(`(?i)\b(bearer\s+)[a-z0-9._~+/=-]{8,}`)
	secretPattern      = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\s*[:=]\s*[^\s,;]+`)
	openAIKeyPattern   = regexp.MustCompile(`\bsk-[a-zA-Z0-9_-]{8,}`)
)

func redactCatalogText(value string) string {
	value = unixPathPattern.ReplaceAllString(value, `${1}[path]`)
	value = homePathPattern.ReplaceAllString(value, `${1}[path]`)
	value = windowsPathPattern.ReplaceAllString(value, `${1}[path]`)
	value = bearerPattern.ReplaceAllString(value, `${1}[redacted]`)
	value = secretPattern.ReplaceAllString(value, `${1}=[redacted]`)
	return openAIKeyPattern.ReplaceAllString(value, `[redacted]`)
}

func sanitizeCatalogSource(value string) string {
	value = strings.TrimSpace(stripComposerControls(value))
	if value == "" || strings.ContainsAny(value, `/\\`) || strings.HasPrefix(value, "~") {
		return ""
	}
	if len(value) >= 2 && unicode.IsLetter(rune(value[0])) && value[1] == ':' {
		return ""
	}
	return redactCatalogText(value)
}

func stripControls(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, value)
}

func stripComposerControls(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func truncateUTF8Bytes(value string, max int) string {
	if len(value) <= max {
		return value
	}
	value = value[:max]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func resolveCatalogExecutable(name string, env []string) string {
	if strings.ContainsRune(name, filepath.Separator) {
		return name
	}
	// Go resolves argv[0] before applying cmd.Env. Search the effective child
	// PATH explicitly so a launchd-started daemon can use the repaired login PATH
	// it passes to providers, even when its original process PATH was minimal.
	for _, entry := range env {
		if !strings.HasPrefix(entry, "PATH=") {
			continue
		}
		for _, dir := range filepath.SplitList(strings.TrimPrefix(entry, "PATH=")) {
			candidate := filepath.Join(dir, name)
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return candidate
			}
		}
		break
	}
	// Parity with interactive provider detection: this also probes ~/.local/bin
	// and Homebrew when the packaged app's PATH still lacks them.
	return binpath.Resolve(name)
}
