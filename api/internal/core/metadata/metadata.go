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
	metadataByte []byte
	metadata     *Metadata
	once         sync.Once
)

// MetadataInfo holds descriptive fields for the Crowbar application.
type MetadataInfo struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Author      string `yaml:"author"`
	URL         string `yaml:"url"`
	License     string `yaml:"license"`
	Copyright   string `yaml:"copyright"`
}

// Version holds the version number and codename.
type Version struct {
	Number   string `yaml:"number"`
	Codename string `yaml:"codename"`
}

// Paths holds path templates for all Crowbar data directories.
type Paths struct {
	Home   OsValue[string] `yaml:"home"`
	Events string          `yaml:"events"`
	Store  string          `yaml:"store"`
	Runs   string          `yaml:"runs"`
	Config string          `yaml:"config"`
	Logs   string          `yaml:"logs"`
}

// Metadata is the top-level structure parsed from metadata.yaml.
type Metadata struct {
	Version  Version      `yaml:"version"`
	Metadata MetadataInfo `yaml:"metadata"`
	Paths    Paths        `yaml:"paths"`
}

// Get returns the singleton Metadata parsed from the embedded metadata.yaml.
// Falls back to defaultMetadata if parsing fails.
func Get() *Metadata {
	once.Do(func() {
		metadata = &Metadata{}
		if err := yaml.Unmarshal(metadataByte, metadata); err != nil {
			metadata = defaultMetadata()
		}
	})
	return metadata
}

// GetVersion returns the application version number.
func GetVersion() string {
	return Get().Version.Number
}

// GetStateDirPath returns the resolved absolute path to the state directory
// (the parent of the events and store directories).
func GetStateDirPath() string {
	return filepath.Dir(GetEventsPath())
}

// GetStateDirPathAt returns the state directory path rooted at homeDir.
func GetStateDirPathAt(
	homeDir string,
) string {
	return filepath.Dir(GetEventsPathAt(homeDir))
}

// GetEventsPath returns the resolved absolute path to the events directory.
func GetEventsPath() string {
	return resolvePath(Get().Paths.Events, resolveHome())
}

// GetEventsPathAt returns the events directory path rooted at homeDir.
func GetEventsPathAt(
	homeDir string,
) string {
	return resolvePath(Get().Paths.Events, homeDir)
}

// GetStorePath returns the resolved absolute path to the GORM store directory.
func GetStorePath() string {
	return resolvePath(Get().Paths.Store, resolveHome())
}

// GetStorePathAt returns the store directory path rooted at homeDir.
func GetStorePathAt(
	homeDir string,
) string {
	return resolvePath(Get().Paths.Store, homeDir)
}

// GetRunsPath returns the resolved absolute path to the agent-run artifacts directory.
func GetRunsPath() string {
	return resolvePath(Get().Paths.Runs, resolveHome())
}

// GetRunsPathAt returns the runs directory path rooted at homeDir.
func GetRunsPathAt(
	homeDir string,
) string {
	return resolvePath(Get().Paths.Runs, homeDir)
}

// GetConfigPath returns the resolved absolute path to the config file.
func GetConfigPath() string {
	return resolvePath(Get().Paths.Config, resolveHome())
}

// GetLogsPath returns the resolved absolute path to the logs directory.
func GetLogsPath() string {
	return resolvePath(Get().Paths.Logs, resolveHome())
}

// GetLogsPathAt returns the logs directory path rooted at homeDir.
func GetLogsPathAt(
	homeDir string,
) string {
	return resolvePath(Get().Paths.Logs, homeDir)
}

func resolvePath(
	tmpl string,
	home string,
) string {
	return filepath.FromSlash(strings.ReplaceAll(tmpl, "{{home}}", home))
}

func resetForTesting() {
	metadata = nil
	once = sync.Once{}
}

func defaultMetadata() *Metadata {
	return &Metadata{
		Version: Version{Number: "0.1.0", Codename: "Crowbar"},
		Metadata: MetadataInfo{
			Name:        "Crowbar",
			Description: "Local, single-user agentic development platform.",
			Author:      "char2cs",
		},
		Paths: Paths{
			Home:   OsValue[string]{Default: "~/.crowbar"},
			Events: "{{home}}/state/events",
			Store:  "{{home}}/state/store",
			Runs:   "{{home}}/runs",
			Config: "{{home}}/config.yaml",
			Logs:   "{{home}}/logs",
		},
	}
}
