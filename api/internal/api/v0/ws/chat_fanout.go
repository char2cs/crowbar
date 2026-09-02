package ws

// ChatFanoutFilter scopes a SHARED-worktree stream (git, review, files, search,
// identity — spec §4.2's shared bucket) to a chat-scoped subscriber: the
// client's own :chatId must be one of the chat ids the PUSHING side resolved
// for the workspace the event describes (spec §7.4).
//
// Membership rather than equality is what makes one Push reach the N sibling
// chats sharing a worktree in a single fan-out pass. Resolving that set at PUSH
// time rather than at connect time is what makes a chat created AFTER a client
// connected still receive events for the worktree it now shares — a client's
// predicate is compiled once, at connect, and only the event can carry news of
// the fan-out.
//
// It is Required, so a subscriber carrying no chat id at all receives nothing.
// A wsId-keyed filter answers that same client the opposite way: it resolves
// empty, goes inactive, and hands it EVERY workspace's events (see
// FilterDef.Required). The stream must also be FlatNamespace — a chat-scoped
// client's namespace scope is its bare chat id, which can never prefix-match an
// event namespaced by workspace.
func ChatFanoutFilter[T any](
	chatIDs func(T) []string,
) FilterDef[T] {
	return FilterDef[T]{
		Param:      "chatId",
		ExtractSet: chatIDs,
		Match:      ExactMatch,
		Required:   true,
	}
}
