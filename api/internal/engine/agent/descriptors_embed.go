package agent

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed descriptors/*.yaml
var embedded embed.FS

// ResolveDescriptor loads provider descriptor by id: a disk override at
// <homeDir>/descriptors/<id>.yaml wins, else the embedded default.
func ResolveDescriptor(homeDir, providerID string) (*Descriptor, error) {
	override := filepath.Join(homeDir, "descriptors", providerID+".yaml")
	if data, err := os.ReadFile(override); err == nil {
		return LoadDescriptor(data)
	}
	data, err := embedded.ReadFile("descriptors/" + providerID + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("agent: unknown provider %q: %w", providerID, err)
	}
	return LoadDescriptor(data)
}
