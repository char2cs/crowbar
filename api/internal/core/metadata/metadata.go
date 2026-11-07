package metadata

import (
	_ "embed"
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

// Metadata is the top-level structure parsed from metadata.yaml.
type Metadata struct {
	Version  Version      `yaml:"version"`
	Metadata MetadataInfo `yaml:"metadata"`
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

func defaultMetadata() *Metadata {
	return &Metadata{
		Version: Version{Number: "0.1.0", Codename: "Crowbar"},
		Metadata: MetadataInfo{
			Name:        "Crowbar",
			Description: "Local, single-user agentic development platform.",
			Author:      "char2cs",
		},
	}
}
