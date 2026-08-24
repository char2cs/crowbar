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
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var (
	ErrUnsupported = errors.New("agents: provider declares no telemetry")

	ErrUnsupportedFormat = errors.New("agents: unsupported telemetry format")
)

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
		if pct, ok := mapping.Float(decoded, w.UsedPercent); ok {
			window.UsedPercent = &pct
		}
		if at, ok := mapping.Time(decoded, w.ResetsAt); ok {
			window.ResetsAt = &at
		}

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
	v, ok := mapping.Int(decoded, path)
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
	v, ok := mapping.Float(decoded, path)
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
	return mapping.String(decoded, path)
}
