package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

func TestMaterializeAttachmentsForDispatch_RewritesAReferencedFile(t *testing.T) {
	home := t.TempDir()
	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")
	fileName, _, err := repoattachments.Store(durableDir, "ab12", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	text := "please look at ![a photo](chats/chat-1/attachments/" + fileName + ") thanks"
	out, err := materializeAttachmentsForDispatch(chatsDir, worktree, "chat-1", "runner-1", text)
	require.NoError(t, err)

	wantRel := ".crowbar-attachments/runner-1/" + fileName
	assert.Contains(t, out, wantRel)
	assert.NotContains(t, out, "chats/chat-1/attachments/"+fileName)
	data, err := os.ReadFile(filepath.Join(worktree, ".crowbar-attachments", "runner-1", fileName))
	require.NoError(t, err)
	assert.Equal(t, "bytes", string(data))
}

func TestMaterializeAttachmentsForDispatch_LeavesUnreferencedTextUntouched(t *testing.T) {
	out, err := materializeAttachmentsForDispatch("chats", "worktree", "chat-1", "runner-1", "plain text, no attachments")
	require.NoError(t, err)
	assert.Equal(t, "plain text, no attachments", out)
}

func TestMaterializeAttachmentsForDispatch_IgnoresAReferenceToADifferentChat(t *testing.T) {
	text := "![x](chats/OTHER-CHAT/attachments/f.png)"
	out, err := materializeAttachmentsForDispatch("chats", "worktree", "chat-1", "runner-1", text)
	require.NoError(t, err)
	assert.Equal(t, text, out, "a reference naming a different chat's store must never be rewritten")
}

func TestMaterializeAttachmentsForDispatch_LeavesAMissingDurableFileUnrewritten(t *testing.T) {
	text := "![gone](chats/chat-1/attachments/never-uploaded.png)"
	out, err := materializeAttachmentsForDispatch(t.TempDir(), t.TempDir(), "chat-1", "runner-1", text)
	require.NoError(t, err)
	assert.Equal(t, text, out)
}

func TestMaterializeAttachmentsForDispatch_MultipleReferences(t *testing.T) {
	home := t.TempDir()
	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")

	fileName1, _, err := repoattachments.Store(durableDir, "id1", "photo1.png", []byte("bytes1"))
	require.NoError(t, err)
	fileName2, _, err := repoattachments.Store(durableDir, "id2", "photo2.png", []byte("bytes2"))
	require.NoError(t, err)

	text := "look at ![](chats/chat-1/attachments/" + fileName1 + ") and ![](chats/chat-1/attachments/" + fileName2 + ")"
	out, err := materializeAttachmentsForDispatch(chatsDir, worktree, "chat-1", "runner-1", text)
	require.NoError(t, err)

	// Both references should be rewritten
	assert.Contains(t, out, ".crowbar-attachments/runner-1/"+fileName1)
	assert.Contains(t, out, ".crowbar-attachments/runner-1/"+fileName2)
	assert.NotContains(t, out, "chats/chat-1/attachments/"+fileName1)
	assert.NotContains(t, out, "chats/chat-1/attachments/"+fileName2)

	// Both files should be in scratch
	_, err = os.ReadFile(filepath.Join(worktree, ".crowbar-attachments", "runner-1", fileName1))
	require.NoError(t, err)
	_, err = os.ReadFile(filepath.Join(worktree, ".crowbar-attachments", "runner-1", fileName2))
	require.NoError(t, err)
}

func TestMaterializeAttachmentsForDispatch_DuplicateReference(t *testing.T) {
	home := t.TempDir()
	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")

	fileName, _, err := repoattachments.Store(durableDir, "id1", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	// Same reference appears twice in the text
	ref := "chats/chat-1/attachments/" + fileName
	text := "![](" + ref + ") and ![](" + ref + ")"
	out, err := materializeAttachmentsForDispatch(chatsDir, worktree, "chat-1", "runner-1", text)
	require.NoError(t, err)

	// Both occurrences should be rewritten
	scratchRef := ".crowbar-attachments/runner-1/" + fileName
	count := 0
	pos := 0
	for {
		idx := findStringIndex(out, scratchRef, pos)
		if idx == -1 {
			break
		}
		count++
		pos = idx + 1
	}
	assert.Equal(t, 2, count, "both occurrences of the same reference should be rewritten")
	assert.NotContains(t, out, ref)
}

func TestMaterializeAttachmentsForDispatch_PartiallyMissingFiles(t *testing.T) {
	home := t.TempDir()
	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")

	fileName1, _, err := repoattachments.Store(durableDir, "id1", "photo1.png", []byte("bytes1"))
	require.NoError(t, err)

	// Reference to existing file and missing file
	text := "![](chats/chat-1/attachments/" + fileName1 + ") and ![](chats/chat-1/attachments/missing.png)"
	out, err := materializeAttachmentsForDispatch(chatsDir, worktree, "chat-1", "runner-1", text)
	require.NoError(t, err)

	// Existing file should be rewritten
	assert.Contains(t, out, ".crowbar-attachments/runner-1/"+fileName1)
	assert.NotContains(t, out, "chats/chat-1/attachments/"+fileName1)

	// Missing file should stay unrewritten
	assert.Contains(t, out, "chats/chat-1/attachments/missing.png")

	// Existing file should be in scratch
	_, err = os.ReadFile(filepath.Join(worktree, ".crowbar-attachments", "runner-1", fileName1))
	require.NoError(t, err)
}

func TestMaterializeAttachmentsForDispatch_MkdirAllError(t *testing.T) {
	// Use a path that can't be created as a directory
	// (e.g., trying to create under a file instead of a directory)
	home := t.TempDir()
	filePath := filepath.Join(home, "file")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")
	fileName, _, err := repoattachments.Store(durableDir, "id1", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	// Create a file where we try to create the scratch directory
	worktreeFile := filePath
	_, err = os.Create(worktreeFile)
	require.NoError(t, err)

	// Try to materialize with the scratch path being under a file
	// This should fail when trying to create the scratch directory
	text := "![](chats/chat-1/attachments/" + fileName + ")"
	_, err = materializeAttachmentsForDispatch(chatsDir, worktreeFile, "chat-1", "runner-1", text)
	require.Error(t, err)
}

func TestMaterializeAttachmentsForDispatch_WriteFileError(t *testing.T) {
	// Create a scenario where we can't write the scratch file
	home := t.TempDir()
	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")

	fileName, _, err := repoattachments.Store(durableDir, "id1", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	scratchDir := worktreepath.AttachmentScratchDir(worktree, "runner-1")
	require.NoError(t, os.MkdirAll(scratchDir, 0o700))

	// Create a file at the destination path so WriteFile fails
	destFile := filepath.Join(scratchDir, fileName)
	require.NoError(t, os.WriteFile(destFile, []byte("old"), 0o700))

	// Now make the file a directory to cause WriteFile to fail
	require.NoError(t, os.Remove(destFile))
	require.NoError(t, os.Mkdir(destFile, 0o700))

	text := "![](chats/chat-1/attachments/" + fileName + ")"
	_, err = materializeAttachmentsForDispatch(chatsDir, worktree, "chat-1", "runner-1", text)
	require.Error(t, err)
}

// findStringIndex finds the index of substr in s starting from startPos, returns -1 if not found.
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
