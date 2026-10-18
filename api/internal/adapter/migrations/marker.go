package migrations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

const (
	// markersDir is where the pre-engine daemon left an empty file per legacy
	// step it finished. Adopting a step is the only reader of a marker.
	markersDir = "upgrades"
	// adoptedKey checkpoints an adoption before the marker is deleted.
	adoptedKey = "adopted-marker"
)

// Adopting is impl for legacy step number, except on a home where an older
// daemon finished the step and left its marker: there the step is adopted
// and impl neither applies nor verifies anything. The adoption is recorded
// before the marker goes, so an attempt stopped in between never re-runs it.
func Adopting(
	number int,
	impl migrate.Impl,
) migrate.Impl {
	name, ok := legacyNames[number]
	if !ok {
		return migrate.Impl{}
	}
	adopted := false
	return migrate.Impl{
		Apply: func(ctx context.Context, env migrate.Env, cp *migrate.Checkpoints) (migrate.Counts, error) {
			var err error
			adopted, err = adopting(env.Home, name, cp.Done)
			if err != nil {
				return nil, err
			}
			if !adopted {
				return impl.Apply(ctx, env, cp)
			}
			return nil, adopt(env.Home, name, cp)
		},
		Verify: func(ctx context.Context, env migrate.Env) error {
			if adopted || impl.Verify == nil {
				return nil
			}
			return impl.Verify(ctx, env)
		},
	}
}

// adopting reports whether the step adopts its marker instead of running: an
// older daemon left the marker, or an attempt recorded the adoption (done) and
// was stopped before the step finished. Plan and Apply decide by it alike.
func adopting(
	h home.Home,
	name string,
	done func(key string) bool,
) (bool, error) {
	present, err := markerPresent(h, name)
	if err != nil {
		return false, err
	}
	return present || done(adoptedKey), nil
}

func adopt(
	h home.Home,
	name string,
	cp *migrate.Checkpoints,
) error {
	if cp.Done(adoptedKey) {
		return removeMarker(h, name)
	}
	if err := cp.Mark(adoptedKey, migrate.Counts{"adopted": 1}); err != nil {
		return err
	}
	return removeMarker(h, name)
}

// markerSubject is the marker's path as a plan shows it, relative to the home.
func markerSubject(
	name string,
) string {
	return path.Join("state", markersDir, name)
}

func markerPresent(
	h home.Home,
	name string,
) (bool, error) {
	_, err := os.Lstat(filepath.Join(h.State(), markersDir, name))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("migrations: marker %s: %w", name, err)
	}
}

// removeMarker deletes the step's marker, then the markers directory once it
// holds nothing else; either may already be gone.
func removeMarker(
	h home.Home,
	name string,
) error {
	dir := filepath.Join(h.State(), markersDir)
	if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("migrations: delete marker %s: %w", name, err)
	}
	left, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(left) > 0) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("migrations: read %s: %w", dir, err)
	}
	if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("migrations: delete %s: %w", dir, err)
	}
	return nil
}
