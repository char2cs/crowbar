// Package telemetry maps a provider's cost and capacity report onto Crowbar
// facts.
//
// It models the FACTS, not the transport. Claude bundles context, cost, rate
// limits and model identity into one payload; another provider will split them
// differently, and a third will report only one of them. Each fact is therefore
// independently optional, and a descriptor's field map is the only thing that
// knows a provider's shape.
//
// Nothing is derived that was not reported. A percentage is computed only when
// capacity and usage are BOTH known, because a wrong gauge is worse than no
// gauge.
package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/env"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/payload"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var (
	// ErrUnsupported reports a descriptor that declares no telemetry at all.
	ErrUnsupported = errors.New("agents: provider declares no telemetry")
	// ErrUnsupportedFormat reports a declared encoding the engine cannot read.
	ErrUnsupportedFormat = errors.New("agents: unsupported telemetry format")
)

// ParseCallback maps a provider-pushed payload.
func ParseCallback(d *spec.Descriptor, raw []byte, now time.Time) (models.Telemetry, error) {
	if d == nil || d.Telemetry == nil || d.Telemetry.Callback == nil {
		return models.Telemetry{}, ErrUnsupported
	}
	cb := d.Telemetry.Callback
	decoded, err := decode(cb.Format, raw)
	if err != nil {
		return models.Telemetry{}, err
	}
	out := mapFacts(cb.Fields, decoded)
	out.RateLimits = mapRateLimits(cb.RateLimits, decoded)
	out.ObservedAt = now
	out.Source = models.TelemetrySourceCallback
	return out, nil
}

// Probe runs the declared telemetry command and maps its output.
//
// It shares the catalogue's bounded runner — timeout, output ceiling,
// process-group kill, shared concurrency permit — because it is the same class of
// operation: a fixed subcommand of a provider CLI, run in the chat's worktree.
func Probe(
	ctx context.Context,
	d *spec.Descriptor,
	opts models.ProbeOptions,
	acquire exec.Acquire,
	now time.Time,
) (models.Telemetry, error) {
	if d == nil || d.Telemetry == nil || d.Telemetry.Probe == nil {
		return models.Telemetry{}, ErrUnsupported
	}
	p := d.Telemetry.Probe
	cwd, err := validWorkdir(opts.Cwd)
	if err != nil {
		return models.Telemetry{}, err
	}

	probeCtx, cancel := context.WithTimeout(
		ctx, time.Duration(p.EffectiveTimeoutMS())*time.Millisecond,
	)
	defer cancel()

	baseEnv := opts.Env
	if baseEnv == nil {
		baseEnv = os.Environ()
	}
	probeEnv := env.Clear(baseEnv, d.Spawn.Env.Clear)
	probeEnv = env.Replace(probeEnv, "PWD", cwd)

	runner := exec.New(exec.Options{
		Executable: exec.Executable(d.Spawn.Cmd, baseEnv),
		Cwd:        cwd,
		Env:        probeEnv,
		MaxStdout:  p.EffectiveMaxStdoutBytes(),
		MaxStderr:  p.EffectiveMaxStderrBytes(),
		Acquire:    acquire,
	})
	raw, err := runner.Run(probeCtx, p.Command)
	if err != nil {
		return models.Telemetry{}, err
	}
	return ParseProbe(d, raw, now)
}

// ErrInvalidWorkdir reports a working directory that is missing or relative.
var ErrInvalidWorkdir = errors.New("agents: telemetry probe worktree is invalid")

func validWorkdir(dir string) (string, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return "", ErrInvalidWorkdir
	}
	clean := filepath.Clean(dir)
	info, statErr := os.Stat(clean)
	if statErr != nil || !info.IsDir() {
		return "", ErrInvalidWorkdir
	}
	return clean, nil
}

// ParseProbe maps the output of a Crowbar-invoked telemetry command.
func ParseProbe(d *spec.Descriptor, raw []byte, now time.Time) (models.Telemetry, error) {
	if d == nil || d.Telemetry == nil || d.Telemetry.Probe == nil {
		return models.Telemetry{}, ErrUnsupported
	}
	probe := d.Telemetry.Probe
	decoded, err := decode(probe.Format, raw)
	if err != nil {
		return models.Telemetry{}, err
	}
	out := mapFacts(probe.Fields, decoded)
	out.ObservedAt = now
	out.Source = models.TelemetrySourceProbe
	return out, nil
}

func decode(format string, raw []byte) (map[string]any, error) {
	if format != "json" {
		return nil, fmt.Errorf("%w %q", ErrUnsupportedFormat, format)
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("agents: parse telemetry payload: %w", err)
	}
	return m, nil
}

// mapFacts reads every declared fact. A fact whose path is unmapped, absent, or
// the wrong shape stays nil, and a group with nothing in it is left nil entirely
// rather than being an empty struct that renders as a zeroed gauge.
func mapFacts(fields map[string]string, decoded map[string]any) models.Telemetry {
	var out models.Telemetry

	capacity := readInt(fields, decoded, spec.FactContextCapacityTokens)
	used := readInt(fields, decoded, spec.FactContextUsedTokens)
	usedPct := readFloat(fields, decoded, spec.FactContextUsedPercent)
	remainPct := readFloat(fields, decoded, spec.FactContextRemainPercent)
	if capacity != nil || used != nil || usedPct != nil || remainPct != nil {
		out.Context = &models.ContextUsage{
			CapacityTokens:   capacity,
			UsedTokens:       used,
			UsedPercent:      usedPct,
			RemainingPercent: remainPct,
		}
		derive(out.Context)
	}

	totalUSD := readFloat(fields, decoded, spec.FactCostTotalUSD)
	apiMS := readInt(fields, decoded, spec.FactCostAPIDurationMS)
	if totalUSD != nil || apiMS != nil {
		out.Cost = &models.SessionCost{TotalUSD: totalUSD, APIDurationMS: apiMS}
	}

	id := readString(fields, decoded, spec.FactModelID)
	name := readString(fields, decoded, spec.FactModelDisplayName)
	if id != "" || name != "" {
		out.Model = &models.ModelIdentity{ID: id, DisplayName: name}
	}
	return out
}

// derive fills the two percentages that follow arithmetically from facts already
// reported, and NOTHING else. Used-percent from capacity and usage is a division
// both of whose operands the provider gave us; remaining-percent is its
// complement. Anything beyond that would be an estimate wearing a gauge's
// clothes.
func derive(c *models.ContextUsage) {
	if c.UsedPercent == nil && c.CapacityTokens != nil && c.UsedTokens != nil && *c.CapacityTokens > 0 {
		pct := float64(*c.UsedTokens) / float64(*c.CapacityTokens) * 100
		c.UsedPercent = &pct
	}
	if c.RemainingPercent == nil && c.UsedPercent != nil {
		remaining := 100 - *c.UsedPercent
		c.RemainingPercent = &remaining
	}
}

func mapRateLimits(windows []spec.TelemetryRateLimitMap, decoded map[string]any) []models.RateLimitWindow {
	out := make([]models.RateLimitWindow, 0, len(windows))
	for _, w := range windows {
		window := models.RateLimitWindow{ID: w.ID, Label: w.Label}
		if pct, ok := payload.Float(decoded, w.UsedPercent); ok {
			window.UsedPercent = &pct
		}
		if at, ok := payload.Time(decoded, w.ResetsAt); ok {
			window.ResetsAt = &at
		}
		// A window the provider did not report this time is omitted rather than
		// carried as an empty row: the UI would render it as a gauge at zero.
		if window.UsedPercent == nil && window.ResetsAt == nil {
			continue
		}
		out = append(out, window)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func readInt(fields map[string]string, decoded map[string]any, fact string) *int {
	path, mapped := fields[fact]
	if !mapped {
		return nil
	}
	v, ok := payload.Int(decoded, path)
	if !ok {
		return nil
	}
	return &v
}

func readFloat(fields map[string]string, decoded map[string]any, fact string) *float64 {
	path, mapped := fields[fact]
	if !mapped {
		return nil
	}
	v, ok := payload.Float(decoded, path)
	if !ok {
		return nil
	}
	return &v
}

func readString(fields map[string]string, decoded map[string]any, fact string) string {
	path, mapped := fields[fact]
	if !mapped {
		return ""
	}
	return payload.String(decoded, path)
}
