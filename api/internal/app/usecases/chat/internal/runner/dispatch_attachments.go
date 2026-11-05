package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
)

// attachmentRefPattern matches a markdown link/image target in the durable store,
// (chats/<chatID>/attachments/<file>). Neither segment may contain "/", which rejects
// traversal; stored names are server-generated without whitespace or brackets.
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

// materializeAttachmentsForDispatch returns text with chatID's attachment refs rewritten
// to absolute store paths; the stored text keeps logical refs. No file is copied (the
// CLIs read outside the worktree unprompted); a ref to a deleted file is left as is.
func materializeAttachmentsForDispatch(chatsDir, chatID, text string) string {
	refs := findAttachmentRefs(chatID, text)
	if len(refs) == 0 {
		return text
	}
	durableDir := worktreepath.AttachmentsDir(chatsDir, chatID)
	out := text
	done := make(map[string]bool, len(refs))
	for _, ref := range refs {
		// ReplaceAll already rewrote every occurrence; a second pass would match the
		// logical ref as the suffix of the absolute path just written and nest it.
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

// rewritePromptTextForDispatch is materializeAttachmentsForDispatch with the chatsDir
// lookup folded in, for a caller that only has a workspaceID.
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
