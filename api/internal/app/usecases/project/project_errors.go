package project

import "errors"

// ErrFolderNotFound is returned when a project import targets a path that does
// not exist on disk. The import validates the path before persisting anything,
// so a nonexistent folder leaves no project behind. Handlers map it to HTTP
// 404 via libs.StatusAndMessage; the message stays free of filesystem
// internals on purpose.
var ErrFolderNotFound = errors.New("folder does not exist")
