// Package agentjournal is the on-disk bookkeeping an agent chat keeps beside
// its own files rather than in a database. Two journals live here: the prompt
// request ledger that makes a React prompt delivery at-most-once across a
// daemon crash, and the hook delivery ledger that makes Crowbar's own hook
// relay exactly-once at the ingress boundary. Both are plain directories of one
// JSON record per id, committed by a single atomic write sequence (record.go):
// what a caller reads back after a crash is always a record that was durably
// written whole.
//
// They are on disk, not in the read model, because both must answer a question
// asked BEFORE any aggregate is touched — "did this exact request already
// happen?" — and must survive the crash that made the question worth asking.
//
// Every method takes its directory as a parameter. A chat's journal location is
// derived from the workspace reader (AgentChatsDir), which does not exist until
// repositories.New returns, so this store cannot resolve its own location and
// never tries to.
package agentjournal

// Option configures a journal at construction.
type Option func(*options)

// WithDirSync replaces the parent-directory fsync run after each atomic rename.
// Production never passes it: the default is a real fsync+close, which is what
// makes a committed record survive power loss. A test passes its own to inject
// a deterministic durability fault at the exact instant a record commits.
func WithDirSync(
	syncDir func(string) error,
) Option {
	return func(o *options) { o.syncDir = syncDir }
}

type options struct {
	syncDir func(string) error
}

func resolve(opts []Option) options {
	resolved := options{syncDir: syncJournalDir}
	for _, opt := range opts {
		opt(&resolved)
	}
	return resolved
}
