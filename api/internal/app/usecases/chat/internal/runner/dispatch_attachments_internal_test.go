package runner

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
)

func TestMaterializeAttachmentsForDispatch_RewritesAReferencedFile(t *testing.T) {
	home := t.TempDir()
	durableDir := worktreepath.AttachmentsDir(home, "chat-1")
	fileName, _, err := repoattachments.Store(durableDir, "ab12", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	text := "please look at ![a photo](chats/chat-1/attachments/" + fileName + ") thanks"
	out := materializeAttachmentsForDispatch(home, "chat-1", text)

	wantAbs := filepath.ToSlash(filepath.Join(durableDir, fileName))
	assert.Contains(t, out, wantAbs, "the rewritten reference must be the file's real absolute path in the durable store")
	assert.True(t, filepath.IsAbs(wantAbs), "sanity: the path this test expects must itself be absolute")
	assert.NotContains(t, out, "]("+"chats/chat-1/attachments/"+fileName+")",
		"the logical markdown link must be gone, replaced by the absolute path")
}

func TestMaterializeAttachmentsForDispatch_LeavesUnreferencedTextUntouched(t *testing.T) {
	out := materializeAttachmentsForDispatch(t.TempDir(), "chat-1", "plain text, no attachments")
	assert.Equal(t, "plain text, no attachments", out)
}

func TestMaterializeAttachmentsForDispatch_IgnoresAReferenceToADifferentChat(t *testing.T) {
	text := "![x](chats/OTHER-CHAT/attachments/f.png)"
	out := materializeAttachmentsForDispatch(t.TempDir(), "chat-1", text)
	assert.Equal(t, text, out, "a reference naming a different chat's store must never be rewritten")
}

func TestMaterializeAttachmentsForDispatch_LeavesAMissingDurableFileUnrewritten(t *testing.T) {
	text := "![gone](chats/chat-1/attachments/never-uploaded.png)"
	out := materializeAttachmentsForDispatch(t.TempDir(), "chat-1", text)
	assert.Equal(t, text, out)
}

func TestMaterializeAttachmentsForDispatch_MultipleReferences(t *testing.T) {
	home := t.TempDir()
	durableDir := worktreepath.AttachmentsDir(home, "chat-1")

	fileName1, _, err := repoattachments.Store(durableDir, "id1", "photo1.png", []byte("bytes1"))
	require.NoError(t, err)
	fileName2, _, err := repoattachments.Store(durableDir, "id2", "photo2.png", []byte("bytes2"))
	require.NoError(t, err)

	text := "look at ![](chats/chat-1/attachments/" + fileName1 + ") and ![](chats/chat-1/attachments/" + fileName2 + ")"
	out := materializeAttachmentsForDispatch(home, "chat-1", text)

	assert.Contains(t, out, filepath.ToSlash(filepath.Join(durableDir, fileName1)))
	assert.Contains(t, out, filepath.ToSlash(filepath.Join(durableDir, fileName2)))
	assert.NotContains(t, out, "]("+"chats/chat-1/attachments/"+fileName1+")")
	assert.NotContains(t, out, "]("+"chats/chat-1/attachments/"+fileName2+")")
}

func TestMaterializeAttachmentsForDispatch_DuplicateReference(t *testing.T) {
	home := t.TempDir()
	durableDir := worktreepath.AttachmentsDir(home, "chat-1")

	fileName, _, err := repoattachments.Store(durableDir, "id1", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	ref := "chats/chat-1/attachments/" + fileName
	text := "![](" + ref + ") and ![](" + ref + ")"
	out := materializeAttachmentsForDispatch(home, "chat-1", text)

	absRef := filepath.ToSlash(filepath.Join(durableDir, fileName))
	count := 0
	pos := 0
	for {
		idx := findStringIndex(out, absRef, pos)
		if idx == -1 {
			break
		}
		count++
		pos = idx + 1
	}
	assert.Equal(t, 2, count, "both occurrences of the same reference should be rewritten")
	assert.NotContains(t, out, "]("+ref+")")
}

func TestMaterializeAttachmentsForDispatch_PartiallyMissingFiles(t *testing.T) {
	home := t.TempDir()
	durableDir := worktreepath.AttachmentsDir(home, "chat-1")

	fileName1, _, err := repoattachments.Store(durableDir, "id1", "photo1.png", []byte("bytes1"))
	require.NoError(t, err)

	text := "![](chats/chat-1/attachments/" + fileName1 + ") and ![](chats/chat-1/attachments/missing.png)"
	out := materializeAttachmentsForDispatch(home, "chat-1", text)

	assert.Contains(t, out, filepath.ToSlash(filepath.Join(durableDir, fileName1)))
	assert.NotContains(t, out, "]("+"chats/chat-1/attachments/"+fileName1+")")

	assert.Contains(t, out, "chats/chat-1/attachments/missing.png")
}

func TestMaterializeAttachmentsForDispatch_RejectsPathTraversalInFileName(t *testing.T) {
	text := "![](chats/chat-1/attachments/../../chat-2/attachments/secret.png)"
	out := materializeAttachmentsForDispatch(t.TempDir(), "chat-1", text)
	assert.Equal(t, text, out, "a filename segment containing / must not match the pattern and must stay unrewritten")
}

func TestMaterializeAttachmentsForDispatch_RejectsAbsolutePathInFileName(t *testing.T) {
	text := "![](chats/chat-1/attachments//etc/passwd)"
	out := materializeAttachmentsForDispatch(t.TempDir(), "chat-1", text)
	assert.Equal(t, text, out, "a filename segment starting with / must not match and must stay unrewritten")
}

func findStringIndex(s, substr string, startPos int) int {
	idx := findIndex([]byte(s[startPos:]), []byte(substr))
	if idx == -1 {
		return -1
	}
	return startPos + idx
}

func findIndex(data, subslice []byte) int {
	for i := 0; i <= len(data)-len(subslice); i++ {
		match := true
		for j := 0; j < len(subslice); j++ {
			if data[i+j] != subslice[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
