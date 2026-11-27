package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
)

var attachmentRefPattern = regexp.MustCompile(`\]\((chats/([^/\s)]+)/attachments/([^/)\s]+))\)`)

type attachmentRef struct {
	logical  string
	fileName string
}

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

func materializeAttachmentsForDispatch(crowbarHome, chatID, text string) string {
	refs := findAttachmentRefs(chatID, text)
	if len(refs) == 0 {
		return text
	}
	durableDir := worktreepath.AttachmentsDir(crowbarHome, chatID)
	out := text
	done := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if done[ref.logical] {
			continue
		}
		done[ref.logical] = true
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

func (rs *Runners) rewritePromptTextForDispatch(
	chatID, text string,
) (string, error) {
	home, err := rs.home()
	if err != nil {
		return "", fmt.Errorf("agent: submit prompt: home: %w", err)
	}
	return materializeAttachmentsForDispatch(home, chatID, text), nil
}
