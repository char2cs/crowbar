package home

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const EnvVar = "CROWBAR_HOME"

var errRelative = errors.New("home: CROWBAR_HOME must be an absolute path")

var errOutsideTemp = errors.New("home: a test binary may only use a home under os.TempDir()")

type Home struct {
	root       string
	overridden bool
}

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

func ResolveDir(
	dir string,
) (Home, error) {
	if dir == "" {
		return Resolve()
	}
	return At(dir)
}

func At(
	root string,
) (Home, error) {
	if !filepath.IsAbs(root) {
		return Home{}, fmt.Errorf("%w: %q", errRelative, root)
	}
	fallback, err := defaultRoot()
	overridden := err != nil || filepath.Clean(root) != filepath.Clean(fallback)
	return newHome(root, overridden)
}

func (h Home) IsDefault() bool {
	return !h.overridden
}

func (h Home) Root() string {
	return h.root
}

func (h Home) State() string {
	return filepath.Join(h.root, "state")
}

func (h Home) Events() string {
	return filepath.Join(h.State(), "events")
}

func (h Home) Content() string {
	return filepath.Join(h.State(), "content")
}

func (h Home) Store() string {
	return filepath.Join(h.State(), "store")
}

func (h Home) Config() string {
	return filepath.Join(h.root, "config.yaml")
}

func (h Home) LayoutFile() string {
	return filepath.Join(h.root, "LAYOUT")
}

func (h Home) LockFile() string {
	return filepath.Join(h.State(), ".lock")
}

func (h Home) DaemonLog() string {
	return filepath.Join(h.root, "logs", "daemon.log")
}

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

func newHome(
	given string,
	overridden bool,
) (Home, error) {
	cleaned := filepath.Clean(given)
	if testing.Testing() && !underDir(os.TempDir(), cleaned) {
		return Home{}, fmt.Errorf("%w: %s", errOutsideTemp, cleaned)
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
