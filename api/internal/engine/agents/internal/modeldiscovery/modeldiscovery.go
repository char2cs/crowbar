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

const (
	maxStderrBytes     = 256 << 10
	connectOutputLimit = 64 << 10
)

var (
	ErrUnsupported     = errors.New("agents: provider declares no model discovery")
	ErrInvalidWorkdir  = errors.New("agents: model discovery worktree is invalid")
	ErrMalformedOutput = errors.New("agents: model discovery output is malformed")
)

type Model struct {
	ID            string
	Label         string
	Efforts       []string
	DefaultEffort string
	Default       bool
}

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
	found := mapModels(rows, itemMapping{
		KeepWhen: disc.KeepWhen, OrderBy: disc.OrderBy, DefaultWhen: disc.DefaultWhen, Item: disc.Item,
	})
	if root, ok := doc.(map[string]any); ok {
		flagCurrent(found, disc.CurrentID, root)
	}
	return found, nil
}

func flagCurrent(found []Model, template string, reply map[string]any) {
	id, ok := expandAll(template, reply)
	if !ok {
		return
	}
	for i := range found {
		found[i].Default = found[i].ID == id
	}
}

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

func expandAll(template string, row map[string]any) (string, bool) {
	complete := template != ""
	out := placeholder.ReplaceAllStringFunc(template, func(m string) string {
		v, ok := scalarString(pathselect.Field(row, m[1:len(m)-1]))
		complete = complete && ok && v != ""
		return v
	})
	return out, complete
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
