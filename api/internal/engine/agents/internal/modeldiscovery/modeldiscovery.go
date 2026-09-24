// Package modeldiscovery is the generic "run a command, decode its JSON,
// keep/order/map rows into models" primitive model.discover: descriptors
// declare. Every field name it reads (item ids, the keep_when/order_by
// paths) is DATA carried in from the descriptor — this package never
// hardcodes a provider's own wire vocabulary.
package modeldiscovery

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

const maxStderrBytes = 256 << 10

var (
	ErrUnsupported     = errors.New("agents: provider declares no model discovery")
	ErrInvalidWorkdir  = errors.New("agents: model discovery worktree is invalid")
	ErrMalformedOutput = errors.New("agents: model discovery output is malformed")
)

// Model is one discovered, already filtered/mapped catalogue entry. Default
// is whether the source itself flagged this row as the provider's default
// model (model.discover.default_when) — false for EVERY row when the
// source states no such thing, never inferred from list position.
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

	runner := newRunner(d, disc, cwd, opts, acquire)
	raw, err := runner.Run(probeCtx, disc.Command)
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

func newRunner(
	d *spec.Descriptor,
	disc *spec.ModelDiscoverSpec,
	cwd string,
	opts models.ProbeOptions,
	acquire exec.Acquire,
) *exec.Runner {
	baseEnv := opts.Env
	if baseEnv == nil {
		baseEnv = os.Environ()
	}
	probeEnv := env.Clear(baseEnv, d.Spawn.Env.Clear)
	probeEnv = env.Replace(probeEnv, "PWD", cwd)
	return exec.New(exec.Options{
		Executable: exec.Executable(d.Spawn.Cmd, baseEnv),
		Cwd:        cwd,
		Env:        probeEnv,
		MaxStdout:  disc.EffectiveMaxStdoutBytes(),
		MaxStderr:  maxStderrBytes,
		Acquire:    acquire,
	})
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
// model.manifest: share — factored out so modeldiscovery's mapping logic is
// written once and both sources read a *spec.ModelDiscoverSpec /
// *spec.ModelManifestSpec into this common shape rather than duplicating
// mapModels/renderModel per source.
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
		rendered, ok := renderModel(c.row, m)
		if !ok {
			continue
		}
		out = append(out, rendered)
	}
	return out
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
