package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

// attachmentRefPattern matches a markdown link/image target pointing at the
// durable attachment store: ![alt](chats/<chatID>/attachments/<file>) or
// [name](chats/<chatID>/attachments/<file>). Attachment filenames are always
// server-generated (attachments.Store strips whitespace/parens/brackets out
// of the original name), and never contain path separators — chatID excludes
// "/" (rejects traversal from the chat name), and filename excludes "/" too
// (rejects traversal via ../ or absolute paths in the filename segment).
var attachmentRefPattern = regexp.MustCompile(`\]\((chats/([^/\s)]+)/attachments/([^/)\s]+))\)`)

type attachmentRef struct {
	logical  string
	fileName string
}

// findAttachmentRefs returns every durable attachment reference in text that
// belongs to chatID. A reference naming a DIFFERENT chat is ignored — it
// names a directory this dispatch has no business reading from.
func findAttachmentRefs(chatID, text string) []attachmentRef {
	var out []attachmentRef
	for _, m := range attachmentRefPattern.FindAllStringSubmatch(text, -1) {
		if m[2] != chatID {
			continue
		}
		out = append(out, attachmentRef{logical: m[1], fileName: m[3]})
	}
	return out
}

// materializeAttachmentsForDispatch returns a COPY of text with every durable
// attachment reference (belonging to chatID) rewritten to that file's real,
// absolute path in the durable store (chatsDir/<chatID>/attachments/<file>)
// — the CLI reads the SAME file Crowbar's own asset-serving endpoint does,
// nothing is copied anywhere. text is never mutated — the stored
// LedgerTurn.Text stays the durable logical reference forever; only this
// dispatch-time copy changes.
//
// No copy step exists here on purpose: both Claude and Codex, at every
// permission level Crowbar offers (guarded/manual through full-auto/auto for
// Claude, workspace-write for Codex), read an arbitrary absolute path outside
// their own worktree with no escalation and no prompt — confirmed live
// against the running daemon, not assumed. Materializing a scratch copy
// INSIDE the worktree used to seem like the only way to guarantee a
// readable path, but it bought nothing a real permission boundary needed and
// cost every attachment-bearing turn an untracked directory sitting in the
// user's own git worktree for as long as the turn ran.
//
// A reference to a file no longer in the durable store (deleted out of band)
// is left unrewritten: the CLI then hits a plain "no such file" reading a
// path that does not resolve, rather than this silently sending a broken
// prompt.
func materializeAttachmentsForDispatch(chatsDir, chatID, text string) string {
	refs := findAttachmentRefs(chatID, text)
	if len(refs) == 0 {
		return text
	}
	durableDir := worktreepath.AttachmentsDir(chatsDir, chatID)
	out := text
	done := make(map[string]bool, len(refs))
	for _, ref := range refs {
		// The same reference can appear more than once in text — rewriting it
		// is a whole-string ReplaceAll, so every occurrence is already handled
		// the first time this logical ref is seen. Skipping the repeat matters
		// here specifically because the rewritten form (an absolute path
		// ending in .../chatID/attachments/fileName) CONTAINS the logical
		// reference as its own suffix: a second, redundant ReplaceAll would
		// match that suffix inside the path just written and nest it again.
		if done[ref.logical] {
			continue
		}
		done[ref.logical] = true
		// Belt-and-suspenders: even though the regex excludes "/", reject any
		// fileName that isn't a bare filename (contains path separators). This
		// guards against future regex changes and makes the guarantee robust.
		if filepath.Base(ref.fileName) != ref.fileName {
			continue
		}
		dest := filepath.Join(durableDir, ref.fileName)
		if _, err := os.Stat(dest); err != nil {
			continue
		}
		out = strings.ReplaceAll(out, ref.logical, filepath.ToSlash(dest))
	}
	return out
}

// rewritePromptTextForDispatch is materializeAttachmentsForDispatch with the
// chatsDir lookup folded in, for a call site that only has a workspaceID —
// submitPromptOverAPI (prompts.go), the live api-push delivery path.
// spawnRunner (spawn.go) already resolves chatsDir as part of spawnPaths for
// its own unrelated reasons and calls materializeAttachmentsForDispatch
// directly with it, so it has no need for this wrapper.
func (rs *Runners) rewritePromptTextForDispatch(
	ctx context.Context,
	workspaceID, chatID, text string,
) (string, error) {
	chatsDir, err := rs.ws.AgentChatsDir(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: submit prompt: chats dir: %w", err)
	}
	return materializeAttachmentsForDispatch(chatsDir, chatID, text), nil
}
