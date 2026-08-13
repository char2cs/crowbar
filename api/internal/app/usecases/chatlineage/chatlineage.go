// Package chatlineage answers the one question the Chats panel's parent edge is
// FOR: which chats does this chat read?
//
// A chat whose ParentID names another CHAT is a thread of it, and reads that
// chat's turns and its chat ancestors' in turn. A chat whose ParentID names a
// FOLDER is merely filed there. Folders hold no turns, so the walk steps
// straight through them: a thread two folders deep under a chat resolves exactly
// the lineage it would sitting directly under it, which is what lets a user
// organise threads without ever changing what one of them reads.
//
// The answer is a list of IDS, never turns, and that is the difference between a
// thread and a fork. Nothing is copied when a thread is created; the ids are
// handed over and the ancestors' conversations are fetched when the agent asks,
// so a thread reads its parent AS IT STANDS at the moment of the question —
// including everything the parent has said since.
package chatlineage

// Walk returns the CHAT ancestors of id, nearest first, and is the single
// definition of what a thread inherits.
//
// Folders are transparent to it by construction: parentOf is followed through
// EVERY row above id whatever kind it is, and isChat decides only whether that
// row is collected on the way past. There is no folder case to forget, because
// there is no folder case.
//
// It takes its two lookups as functions rather than reading a store because it
// has two callers standing in front of two different worlds. The read path
// resolves rows from the chat and folder tables. A MOVE resolves them from the
// in-memory plan of a tree that has not been written yet — it has to, since the
// lineage a drag is about to create exists nowhere else, and a store re-read
// would answer with the tree as it was before the drag. Sharing the traversal
// rather than the source is the only way both can agree on what transparency
// means.
//
// The visited set bounds a corrupt parent chain instead of spinning on it. The
// usecase's own cycle guard makes one unrepresentable, but a lineage read is a
// bad place to discover otherwise by hanging the daemon that noticed.
func Walk(
	id string,
	parentOf func(string) string,
	isChat func(string) bool,
) []string {
	var out []string
	seen := map[string]bool{id: true}
	for at := parentOf(id); at != "" && !seen[at]; at = parentOf(at) {
		seen[at] = true
		if isChat(at) {
			out = append(out, at)
		}
	}
	return out
}
