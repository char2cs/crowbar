package agentactivity

import "errors"

// ErrNotFound reports a conversation record that does not exist. It is also what
// a missing content payload maps to, because retention may legitimately have
// swept one.
var ErrNotFound = errors.New("agentactivity: not found")
