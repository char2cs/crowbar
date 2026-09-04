package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
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

// materializeAttachmentsForDispatch copies every attachment text references
// (belonging to chatID) from the durable store (chatsDir/<chatID>/attachments)
// into runnerID's scratch directory inside worktree, and returns a COPY of
// text with each durable reference rewritten to the scratch path. text is
// never mutated — the stored LedgerTurn.Text stays the durable reference
// forever; only this dispatch-time copy changes.
//
// A reference to a file no longer in the durable store (deleted out of band)
// is left unrewritten: the CLI then hits a plain "no such file" reading a
// path that does not resolve, rather than this silently sending a broken
// prompt.
func materializeAttachmentsForDispatch(
	chatsDir, worktree, chatID, runnerID, text string,
) (string, error) {
	refs := findAttachmentRefs(chatID, text)
	if len(refs) == 0 {
		return text, nil
	}
	scratchDir := worktreepath.AttachmentScratchDir(worktree, runnerID)
	//nolint:gosec // G301: 0o700 matches RunnerDir's own perms for daemon-managed chat state.
	if err := os.MkdirAll(scratchDir, 0o700); err != nil {
		return "", fmt.Errorf("agent: materialize attachments: mkdir scratch dir: %w", err)
	}
	durableDir := worktreepath.AttachmentsDir(chatsDir, chatID)
	out := text
	for _, ref := range refs {
		// Belt-and-suspenders: even though the regex excludes "/", reject any
		// fileName that isn't a bare filename (contains path separators). This
		// guards against future regex changes and makes the guarantee robust.
		if filepath.Base(ref.fileName) != ref.fileName {
			continue
		}
		data, _, err := repoattachments.Read(durableDir, ref.fileName)
		if err != nil {
			continue
		}
		dest := filepath.Join(scratchDir, ref.fileName)
		//nolint:gosec // G306: read back only by the CLI subprocess this daemon just forked; matches the durable store's own perms.
		if err := os.WriteFile(dest, data, 0o600); err != nil {
			return "", fmt.Errorf("agent: materialize attachments: write scratch copy: %w", err)
		}
		rel := filepath.ToSlash(filepath.Join(worktreepath.AttachmentScratchDirName, runnerID, ref.fileName))
		out = strings.ReplaceAll(out, ref.logical, rel)
	}
	return out, nil
}

// rewritePromptTextForDispatch is materializeAttachmentsForDispatch with the
// chatsDir lookup folded in, for a call site that only has a workspaceID —
// submitPromptOverAPI (prompts.go), the live api-push delivery path.
// spawnRunner (spawn.go) already resolves chatsDir as part of spawnPaths for
// its own unrelated reasons and calls materializeAttachmentsForDispatch
// directly with it, so it has no need for this wrapper.
func (rs *Runners) rewritePromptTextForDispatch(
	ctx context.Context,
	workspaceID, chatID, worktree, runnerID, text string,
) (string, error) {
	chatsDir, err := rs.ws.AgentChatsDir(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: submit prompt: chats dir: %w", err)
	}
	return materializeAttachmentsForDispatch(chatsDir, worktree, chatID, runnerID, text)
}
