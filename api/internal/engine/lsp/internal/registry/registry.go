package registry

import "path/filepath"

type ServerSpec struct {
	Command               string
	Args                  []string
	LanguageID            string
	Extensions            []string
	InitializationOptions map[string]any
}

type Registry interface {
	ForFile(path string) (ServerSpec, bool)
}

type registry struct {
	index map[string]ServerSpec
}

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

func buildDefaults() map[string]ServerSpec {
	goSpec := ServerSpec{
		Command:               "gopls",
		LanguageID:            "go",
		Extensions:            []string{".go"},
		InitializationOptions: map[string]any{"semanticTokens": true},
	}
	tsSpec := ServerSpec{
		Command:    "typescript-language-server",
		Args:       []string{"--stdio"},
		LanguageID: "typescript",
		Extensions: []string{".ts", ".tsx", ".js", ".jsx"},
	}
	pySpec := ServerSpec{
		Command:    "pyright-langserver",
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
