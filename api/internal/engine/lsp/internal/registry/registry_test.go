package registry_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_DefaultGo(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("main.go")
	require.True(t, ok)
	assert.Equal(t, "gopls", spec.Command)
	assert.Equal(t, "go", spec.LanguageID)
}

func TestRegistry_DefaultTypeScriptTS(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("app.ts")
	require.True(t, ok)
	assert.Equal(t, "typescript-language-server", spec.Command)
	assert.Equal(t, "typescript", spec.LanguageID)
}

func TestRegistry_DefaultTypeScriptTSX(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("component.tsx")
	require.True(t, ok)
	assert.Equal(t, "typescript-language-server", spec.Command)
	assert.Equal(t, "typescript", spec.LanguageID)
}

func TestRegistry_DefaultTypeScriptJS(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("index.js")
	require.True(t, ok)
	assert.Equal(t, "typescript-language-server", spec.Command)
	assert.Equal(t, "typescript", spec.LanguageID)
}

func TestRegistry_DefaultTypeScriptJSX(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("app.jsx")
	require.True(t, ok)
	assert.Equal(t, "typescript-language-server", spec.Command)
	assert.Equal(t, "typescript", spec.LanguageID)
}

func TestRegistry_DefaultPython(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("script.py")
	require.True(t, ok)
	assert.Equal(t, "pyright", spec.Command)
	assert.Equal(t, "python", spec.LanguageID)
}

func TestRegistry_DefaultJava(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("Main.java")
	require.True(t, ok)
	assert.Equal(t, "jdtls", spec.Command)
	assert.Equal(t, "java", spec.LanguageID)
}

func TestRegistry_DefaultC(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("main.c")
	require.True(t, ok)
	assert.Equal(t, "clangd", spec.Command)
	assert.Equal(t, "c", spec.LanguageID)
}

func TestRegistry_DefaultCPP(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("main.cpp")
	require.True(t, ok)
	assert.Equal(t, "clangd", spec.Command)
	assert.Equal(t, "c", spec.LanguageID)
}

func TestRegistry_DefaultCC(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("main.cc")
	require.True(t, ok)
	assert.Equal(t, "clangd", spec.Command)
	assert.Equal(t, "c", spec.LanguageID)
}

func TestRegistry_DefaultH(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("types.h")
	require.True(t, ok)
	assert.Equal(t, "clangd", spec.Command)
	assert.Equal(t, "c", spec.LanguageID)
}

func TestRegistry_DefaultHPP(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("types.hpp")
	require.True(t, ok)
	assert.Equal(t, "clangd", spec.Command)
	assert.Equal(t, "c", spec.LanguageID)
}

func TestRegistry_UnknownExtension(t *testing.T) {
	_, ok := registry.New(nil).ForFile("file.unknownext")
	assert.False(t, ok)
}

func TestRegistry_NoExtension(t *testing.T) {
	_, ok := registry.New(nil).ForFile("Makefile")
	assert.False(t, ok)
}

func TestRegistry_ConfigOverride(t *testing.T) {
	overrides := map[string]registry.ServerSpec{
		".go": {
			Command:    "gopls-custom",
			LanguageID: "go",
			Extensions: []string{".go"},
		},
	}
	r := registry.New(overrides)
	spec, ok := r.ForFile("x.go")
	require.True(t, ok)
	assert.Equal(t, "gopls-custom", spec.Command)
	assert.Equal(t, "go", spec.LanguageID)
}

func TestRegistry_ConfigOverrideNewExtension(t *testing.T) {
	overrides := map[string]registry.ServerSpec{
		".rb": {
			Command:    "solargraph",
			Args:       []string{"stdio"},
			LanguageID: "ruby",
			Extensions: []string{".rb"},
		},
	}
	r := registry.New(overrides)
	spec, ok := r.ForFile("app.rb")
	require.True(t, ok)
	assert.Equal(t, "solargraph", spec.Command)
	assert.Equal(t, "ruby", spec.LanguageID)
	assert.Equal(t, []string{"stdio"}, spec.Args)
}

func TestRegistry_OverrideDoesNotAffectOtherDefaults(t *testing.T) {
	overrides := map[string]registry.ServerSpec{
		".go": {
			Command:    "gopls-custom",
			LanguageID: "go",
			Extensions: []string{".go"},
		},
	}
	r := registry.New(overrides)
	spec, ok := r.ForFile("script.py")
	require.True(t, ok)
	assert.Equal(t, "pyright", spec.Command)
}

func TestRegistry_ForFileWithDirectoryPath(t *testing.T) {
	r := registry.New(nil)
	spec, ok := r.ForFile("/home/user/project/main.go")
	require.True(t, ok)
	assert.Equal(t, "gopls", spec.Command)
}
