// Package registry maps file extensions to language-server commands. It ships
// defaults for Go (gopls), TypeScript/JavaScript (typescript-language-server),
// Python (pyright), Java (jdtls), and C/C++ (clangd), and accepts caller
// overrides to replace or extend any entry (10 §4).
package registry

import "path/filepath"

// ServerSpec describes how to launch a language server.
type ServerSpec struct {
	// Command is the executable name (looked up on PATH).
	Command string
	// Args are extra command-line arguments passed to the server.
	Args []string
	// LanguageID is the LSP language identifier sent in textDocument items.
	LanguageID string
	// Extensions lists the file extensions this spec handles (e.g. ".go").
	Extensions []string
}

// Registry resolves a file path to the ServerSpec that handles it.
type Registry interface {
	// ForFile returns the ServerSpec for the given file path and true, or the
	// zero ServerSpec and false if no server handles the extension.
	ForFile(path string) (ServerSpec, bool)
}

type registry struct {
	index map[string]ServerSpec
}

// New returns a Registry populated with built-in defaults, with any entries in
// overrides applied on top. An override keyed by extension (e.g. ".go")
// replaces the default for that extension; unknown keys extend the registry.
func New(
	overrides map[string]ServerSpec,
) Registry {
	index := buildDefaults()
	applyOverrides(index, overrides)
	return &registry{index: index}
}

func (r *registry) ForFile(
	path string,
) (ServerSpec, bool) {
	ext := filepath.Ext(path)
	if ext == "" {
		return ServerSpec{}, false
	}
	spec, ok := r.index[ext]
	return spec, ok
}

// buildDefaults constructs the built-in extension→ServerSpec map.
func buildDefaults() map[string]ServerSpec {
	goSpec := ServerSpec{
		Command:    "gopls",
		LanguageID: "go",
		Extensions: []string{".go"},
	}
	tsSpec := ServerSpec{
		Command:    "typescript-language-server",
		Args:       []string{"--stdio"},
		LanguageID: "typescript",
		Extensions: []string{".ts", ".tsx", ".js", ".jsx"},
	}
	pySpec := ServerSpec{
		Command:    "pyright",
		Args:       []string{"--stdio"},
		LanguageID: "python",
		Extensions: []string{".py"},
	}
	javaSpec := ServerSpec{
		Command:    "jdtls",
		LanguageID: "java",
		Extensions: []string{".java"},
	}
	cSpec := ServerSpec{
		Command:    "clangd",
		LanguageID: "c",
		Extensions: []string{".c", ".cpp", ".cc", ".h", ".hpp"},
	}

	defaults := []ServerSpec{goSpec, tsSpec, pySpec, javaSpec, cSpec}
	index := make(map[string]ServerSpec)

	for _, spec := range defaults {
		for _, ext := range spec.Extensions {
			index[ext] = spec
		}
	}
	return index
}

// applyOverrides writes each override into index, keyed by its Extensions
// list. If Extensions is empty, the override is keyed by the map key itself.
func applyOverrides(
	index map[string]ServerSpec,
	overrides map[string]ServerSpec,
) {
	for key, spec := range overrides {
		exts := spec.Extensions
		if len(exts) == 0 {
			exts = []string{key}
		}
		for _, ext := range exts {
			index[ext] = spec
		}
	}
}
