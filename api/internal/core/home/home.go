// Package home is the single resolver of the Crowbar home directory and of
// the paths and daemon socket derived from it.
package home

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// EnvVar names the environment variable that relocates the Crowbar home.
const EnvVar = "CROWBAR_HOME"

// ErrRelative is returned when the requested home is not an absolute path.
var ErrRelative = errors.New("home: CROWBAR_HOME must be an absolute path")

// ErrOutsideTemp is returned inside a test binary for a home that is not
// under os.TempDir().
var ErrOutsideTemp = errors.New("home: a test binary may only use a home under os.TempDir()")

// Home is a validated Crowbar home directory. It only computes paths; it
// never creates or touches anything on disk.
type Home struct {
	root       string
	overridden bool
}

// Resolve returns the home named by CROWBAR_HOME when it is set, otherwise
// the default home, .crowbar under the user's home directory.
func Resolve() (Home, error) {
	override := os.Getenv(EnvVar)
	if override != "" {
		return At(override)
	}
	root, err := defaultRoot()
	if err != nil {
		return Home{}, err
	}
	return newHome(root, false)
}

// At returns the home rooted at root, which must be an absolute path. A root
// that differs from the default home counts as an override.
func At(
	root string,
) (Home, error) {
	if !filepath.IsAbs(root) {
		return Home{}, fmt.Errorf("%w: %q", ErrRelative, root)
	}
	fallback, err := defaultRoot()
	overridden := err != nil || filepath.Clean(root) != filepath.Clean(fallback)
	return newHome(root, overridden)
}

// Root is the cleaned home directory.
func (h Home) Root() string {
	return h.root
}

// State is the directory holding the daemon's databases.
func (h Home) State() string {
	return filepath.Join(h.root, "state")
}

// Events is the directory holding the event stream.
func (h Home) Events() string {
	return filepath.Join(h.State(), "events")
}

// Store is the directory holding the view store.
func (h Home) Store() string {
	return filepath.Join(h.State(), "store")
}

// Config is the path of the user configuration file.
func (h Home) Config() string {
	return filepath.Join(h.root, "config.yaml")
}

// LayoutFile is the path of the file recording the on-disk layout version.
func (h Home) LayoutFile() string {
	return filepath.Join(h.root, "LAYOUT")
}

// LockFile is the path of the file that serialises access to the home.
func (h Home) LockFile() string {
	return filepath.Join(h.State(), ".lock")
}

// MigrateDir is the directory holding migration run records.
func (h Home) MigrateDir() string {
	return filepath.Join(h.root, "migrate")
}

func defaultRoot() (string, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home: default root: %w", err)
	}
	return filepath.Join(userHome, ".crowbar"), nil
}

// newHome is the only constructor. Inside a test binary it refuses any home
// outside os.TempDir(), so no test can ever run against the real home.
func newHome(
	root string,
	overridden bool,
) (Home, error) {
	cleaned := filepath.Clean(root)
	if testing.Testing() && !underDir(os.TempDir(), cleaned) {
		return Home{}, fmt.Errorf("%w: %s", ErrOutsideTemp, cleaned)
	}
	return Home{root: cleaned, overridden: overridden}, nil
}

func underDir(
	dir string,
	path string,
) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
