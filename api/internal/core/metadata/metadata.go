package metadata

import (
	_ "embed"
	"path/filepath"
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v3"
)

var (
	//go:embed metadata.yaml
	metadataBytes []byte

	md   *Metadata
	once sync.Once
)

// Metadata holds the application name, version, and home directory template.
type Metadata struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Home    string `yaml:"home"`
}

func get() *Metadata {
	once.Do(func() {
		md = &Metadata{}
		if err := yaml.Unmarshal(metadataBytes, md); err != nil {
			md = defaultMetadata()
		}
	})
	return md
}

func defaultMetadata() *Metadata {
	return &Metadata{
		Name:    "crowbar",
		Version: "v0",
		Home:    "{{home}}/.crowbar",
	}
}

// GetHome returns the resolved ~/.crowbar path using the process home directory.
func GetHome() string {
	return resolveHome(get().Home)
}

// GetHomeAt returns the crowbar home path rooted at homeDir instead of the
// process-level home directory. Use this in tests for isolation.
func GetHomeAt(homeDir string) string {
	return resolvePath(get().Home, homeDir)
}

// resolvePath replaces {{home}} in the template with the given home directory
// and normalizes path separators to the OS-native form.
func resolvePath(tmpl, home string) string {
	return filepath.FromSlash(strings.ReplaceAll(tmpl, "{{home}}", home))
}

// resetForTesting resets the singleton so tests can re-initialize it.
func resetForTesting() {
	md = nil
	once = sync.Once{}
}
