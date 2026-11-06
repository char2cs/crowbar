// Package lineage answers which chats a chat reads: a thread (ParentID names a chat) reads its chat
// ancestors' turns, stepping through folders, which hold none. It returns ids, never copied turns,
// so a thread reads its parent as it stands when the agent asks.
package lineage

// Walk returns the chat ancestors of id, nearest first: parentOf is followed through every row, and isChat
// only decides which are collected. The lookups are functions so a move can walk its unwritten in-memory
// plan with the same traversal the read path uses; the visited set bounds a corrupt parent chain.
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
