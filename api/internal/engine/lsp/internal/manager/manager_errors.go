package manager

import "errors"

// ErrNoServer is returned by ServerForFile when the registry has no spec for
// the file's extension, or the language server binary is not installed. It is
// not a hard failure — callers should surface an empty/degraded result.
var ErrNoServer = errors.New("lsp manager: no server for file")
