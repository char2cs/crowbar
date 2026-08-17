// Package catalog runs a provider's deterministic capability inventory.
//
// It starts no shell, reads no provider-owned files, retains no cache, and
// discards stdout and stderr once the normalised result is built. What comes back
// is provider-neutral and safe to publish: labels, insert text and a source name,
// with every path and credential-shaped string already removed.
package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog/internal/adapters"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog/internal/normalize"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/env"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var (
	// ErrUnsupported reports a provider that declares no catalogue.
	ErrUnsupported = errors.New("agents: provider declares no slash catalog")
	// ErrInvalidWorkdir reports a working directory that is missing or relative. A
	// probe runs a provider CLI, and where it runs changes what it reports.
	ErrInvalidWorkdir = errors.New("agents: catalog worktree is invalid")
	// ErrTimeout reports the probe exceeding its declared budget.
	ErrTimeout = errors.New("agents: catalog probe timed out")
	// ErrMalformedOutput is re-exported so callers match one sentinel.
	ErrMalformedOutput = adapters.ErrMalformedOutput
)

// Probe executes the descriptor-declared pipeline in the given worktree.
func Probe(
	ctx context.Context,
	d *spec.Descriptor,
	opts models.ProbeOptions,
	acquire exec.Acquire,
) (models.SlashCatalog, error) {
	if d == nil || d.Presentation.SlashCatalog == nil {
		return models.SlashCatalog{}, ErrUnsupported
	}
	cwd, err := validWorkdir(opts.Cwd)
	if err != nil {
		return models.SlashCatalog{}, err
	}
	sc := d.Presentation.SlashCatalog
	adapter, ok := adapters.For(sc.Pipeline.Adapter)
	if !ok {
		return models.SlashCatalog{}, ErrMalformedOutput
	}

	probeCtx, cancel := context.WithTimeout(
		ctx, time.Duration(sc.EffectiveTimeoutMS())*time.Millisecond,
	)
	defer cancel()

	return assemble(ctx, probeCtx, d, sc, adapter, newRunner(d, sc, cwd, opts, acquire))
}

// ProbeWith runs the declared pipeline against a caller-supplied runner.
//
// It is the seam a test drives: the adapters, the item mapping and the
// normalisation are exactly the production ones, and only the subprocess is
// replaced. Probe is ProbeWith plus workdir validation and the bounded runner.
func ProbeWith(
	ctx context.Context,
	d *spec.Descriptor,
	runner models.Runner,
) (models.SlashCatalog, error) {
	if d == nil || d.Presentation.SlashCatalog == nil {
		return models.SlashCatalog{}, ErrUnsupported
	}
	sc := d.Presentation.SlashCatalog
	adapter, ok := adapters.For(sc.Pipeline.Adapter)
	if !ok {
		return models.SlashCatalog{}, ErrMalformedOutput
	}
	probeCtx, cancel := context.WithTimeout(
		ctx, time.Duration(sc.EffectiveTimeoutMS())*time.Millisecond,
	)
	defer cancel()
	return assemble(ctx, probeCtx, d, sc, adapter, runner)
}

func assemble(
	ctx, probeCtx context.Context,
	d *spec.Descriptor,
	sc *spec.SlashCatalogSpec,
	adapter adapters.Adapter,
	runner models.Runner,
) (models.SlashCatalog, error) {
	result, err := adapter.Probe(probeCtx, sc, runner)
	if err != nil {
		return models.SlashCatalog{}, classify(ctx, probeCtx, err)
	}

	out := models.SlashCatalog{
		ProviderID:   d.ID,
		Completeness: string(sc.Completeness),
		Warnings:     normalize.Warnings(completenessWarnings(sc.Completeness), result.Warnings...),
	}
	items, warnings := mapItems(result.Candidates, sc.Pipeline.Item, sc.EffectiveMaxItems())
	out.Items = items
	out.Warnings = normalize.Warnings(out.Warnings, warnings...)
	return out, nil
}

func newRunner(
	d *spec.Descriptor,
	sc *spec.SlashCatalogSpec,
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
		MaxStdout:  sc.EffectiveMaxStdoutBytes(),
		MaxStderr:  sc.EffectiveMaxStderrBytes(),
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

// classify separates "the probe ran out of its own budget" from "the caller went
// away". They look identical at the error site and mean opposite things to the
// user: one is a slow provider, the other is a menu they already closed.
func classify(parent, probe context.Context, err error) error {
	if errors.Is(probe.Err(), context.DeadlineExceeded) && parent.Err() == nil {
		return ErrTimeout
	}
	if parent.Err() != nil {
		return parent.Err()
	}
	return err
}

func completenessWarnings(c spec.CatalogCompleteness) []string {
	switch c {
	case spec.CatalogCompletenessModelVisible:
		return []string{
			"This catalog contains model-visible skills and may differ from the provider's native menu.",
		}
	case spec.CatalogCompletenessPluginOnly:
		return []string{
			"This catalog contains enabled plugin skills only; standalone skills may be available in the native terminal.",
		}
	case spec.CatalogCompletenessComplete:
		// A complete inventory needs no caveat, which is the entire meaning of the
		// label: it is the one value that promises the menu is the whole menu.
		return []string{}
	default:
		return []string{}
	}
}
