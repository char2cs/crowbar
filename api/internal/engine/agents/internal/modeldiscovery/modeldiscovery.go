// Package modeldiscovery is the generic model.discover: primitive: run a command, decode its JSON,
// keep/order/map rows into models. Every field name it reads is data from the descriptor, so it
// never hardcodes a provider's own wire vocabulary.
package modeldiscovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/env"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/pathselect"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// connectOutputLimit bounds what a connect command may print: one small JSON object.
const (
	maxStderrBytes     = 256 << 10
	connectOutputLimit = 64 << 10
)

var (
	ErrUnsupported     = errors.New("agents: provider declares no model discovery")
	ErrInvalidWorkdir  = errors.New("agents: model discovery worktree is invalid")
	ErrMalformedOutput = errors.New("agents: model discovery output is malformed")
)

// Model is one discovered, filtered/mapped catalogue entry. Default is whether the
// source flagged this row as default (default_when), never inferred from position.
type Model struct {
	ID            string
	Label         string
	Efforts       []string
	DefaultEffort string
	Default       bool
}

// Probe runs a descriptor's model.discover command and maps its output into
// the kept, ordered model list. It never reads or writes a cache — that is
// Cache's job.
func Probe(
	ctx context.Context,
	d *spec.Descriptor,
	opts models.ProbeOptions,
	acquire exec.Acquire,
) ([]Model, error) {
	if d == nil || d.Model == nil || d.Model.Discover == nil {
		return nil, ErrUnsupported
	}
	disc := d.Model.Discover
	cwd, err := validWorkdir(opts.Cwd)
	if err != nil {
		return nil, err
	}

	probeCtx, cancel := context.WithTimeout(
		ctx, time.Duration(disc.EffectiveTimeoutMS())*time.Millisecond,
	)
	defer cancel()

	raw, err := fetch(probeCtx, d, disc, cwd, opts, acquire)
	if err != nil {
		return nil, err
	}

	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, ErrMalformedOutput
	}
	rows := pathselect.Select([]any{doc}, disc.ItemsPath)
	return mapModels(rows, itemMapping{
		KeepWhen: disc.KeepWhen, OrderBy: disc.OrderBy, DefaultWhen: disc.DefaultWhen, Item: disc.Item,
	}), nil
}

// fetch is the raw catalogue: the descriptor's api call over its own connection, or
// its command's stdout.
func fetch(
	ctx context.Context,
	d *spec.Descriptor,
	disc *spec.ModelDiscoverSpec,
	cwd string,
	opts models.ProbeOptions,
	acquire exec.Acquire,
) ([]byte, error) {
	if disc.API == nil {
		return newRunner(d, disc, cwd, opts, acquire).Run(ctx, disc.Command)
	}
	ep := protocol.Endpoint{
		Argv: d.Runtime.API.Connect,
		Env:  probeEnv(d, cwd, opts),
		Run: func(ctx context.Context, argv, environ []string) ([]byte, error) {
			return exec.New(exec.Options{
				Executable: exec.Executable(argv[0], environ), Cwd: cwd, Env: environ,
				MaxStdout: connectOutputLimit, MaxStderr: maxStderrBytes, Acquire: acquire,
			}).Run(ctx, argv[1:])
		},
	}
	raw, err := protocol.CallOnce(ctx, d, ep, disc.API.Call, disc.API.Params)
	if err != nil {
		return nil, fmt.Errorf("modeldiscovery: %s: %w", d.ID, err)
	}
	if len(raw) > disc.EffectiveMaxStdoutBytes() {
		return nil, ErrMalformedOutput
	}
	return raw, nil
}

func newRunner(
	d *spec.Descriptor,
	disc *spec.ModelDiscoverSpec,
	cwd string,
	opts models.ProbeOptions,
	acquire exec.Acquire,
) *exec.Runner {
	return exec.New(exec.Options{
		Executable: exec.Executable(d.Spawn.Cmd, baseEnvOf(opts)),
		Cwd:        cwd,
		Env:        probeEnv(d, cwd, opts),
		MaxStdout:  disc.EffectiveMaxStdoutBytes(),
		MaxStderr:  maxStderrBytes,
		Acquire:    acquire,
	})
}

func baseEnvOf(opts models.ProbeOptions) []string {
	if opts.Env == nil {
		return os.Environ()
	}
	return opts.Env
}

func probeEnv(d *spec.Descriptor, cwd string, opts models.ProbeOptions) []string {
	return env.Replace(env.Clear(baseEnvOf(opts), d.Spawn.Env.Clear), "PWD", cwd)
}

func validWorkdir(dir string) (string, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return "", ErrInvalidWorkdir
	}
	clean := filepath.Clean(dir)
	info, err := os.Stat(clean)
	if err != nil || !info.IsDir() {
		return "", ErrInvalidWorkdir
	}
	return clean, nil
}

// itemMapping is the filter/order/render vocabulary model.discover: and
// model.manifest: share, so mapModels/renderModel serve both sources.
type itemMapping struct {
	KeepWhen    *spec.ModelFieldMatch
	OrderBy     string
	DefaultWhen *spec.ModelFieldMatch
	Item        spec.ModelItemMapping
}

func mapModels(rows []any, m itemMapping) []Model {
	type candidate struct {
		row   map[string]any
		order float64
	}
	kept := make([]candidate, 0, len(rows))
	for _, v := range rows {
		row, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if !fieldMatches(row, m.KeepWhen) {
			continue
		}
		order, _ := numericOf(pathselect.Field(row, m.OrderBy))
		kept = append(kept, candidate{row: row, order: order})
	}
	if m.OrderBy != "" {
		sort.SliceStable(kept, func(i, j int) bool { return kept[i].order < kept[j].order })
	}

	out := make([]Model, 0, len(kept))
	for _, c := range kept {
		for _, row := range subRows(c.row, m.Item.Each) {
			if rendered, ok := renderModel(row, m); ok {
				out = append(out, rendered)
			}
		}
	}
	return out
}

// subRows is row itself, or with each declared one row per element at that path,
// carrying row as "parent" so a child can name the group it belongs to.
func subRows(row map[string]any, each string) []map[string]any {
	if each == "" {
		return []map[string]any{row}
	}
	var out []map[string]any
	for _, v := range pathselect.Select([]any{row}, each) {
		child, ok := v.(map[string]any)
		if !ok {
			child = map[string]any{"value": v}
		}
		out = append(out, mergedRow(child, row))
	}
	return out
}

func mergedRow(child, parent map[string]any) map[string]any {
	merged := make(map[string]any, len(child)+1)
	maps.Copy(merged, child)
	merged["parent"] = parent
	return merged
}

// fieldMatches reports whether m's field equals its value on row — true for
// a nil m, so an undeclared keep_when keeps every row unconditionally.
func fieldMatches(row map[string]any, m *spec.ModelFieldMatch) bool {
	if m == nil {
		return true
	}
	got, ok := scalarString(pathselect.Field(row, m.Field))
	want, wantOK := scalarString(m.Equals)
	return ok && wantOK && got == want
}

func renderModel(row map[string]any, m itemMapping) (Model, bool) {
	item := m.Item
	id := expand(item.ID, row)
	label := expand(item.Label, row)
	if id == "" || label == "" {
		return Model{}, false
	}
	return Model{
		ID:            id,
		Label:         label,
		DefaultEffort: expand(item.DefaultEffort, row),
		Efforts:       stringsOf(pathselect.Select([]any{row}, item.Efforts)),
		Default:       m.DefaultWhen != nil && fieldMatches(row, m.DefaultWhen),
	}, true
}

var placeholder = regexp.MustCompile(`\{([^{}]+)\}`)

// expand replaces every {field} in template with row's own value at that
// field path — the arbitrary-field-name counterpart of catalog/mapping.go's
// expand, which only ever substitutes a fixed handful of names.
func expand(template string, row map[string]any) string {
	if template == "" {
		return ""
	}
	return placeholder.ReplaceAllStringFunc(template, func(m string) string {
		field := m[1 : len(m)-1]
		v, ok := scalarString(pathselect.Field(row, field))
		if !ok {
			return ""
		}
		return v
	})
}

func scalarString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case bool:
		return strconv.FormatBool(t), true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	default:
		return "", false
	}
}

func numericOf(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func stringsOf(vals []any) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if s, ok := scalarString(v); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
