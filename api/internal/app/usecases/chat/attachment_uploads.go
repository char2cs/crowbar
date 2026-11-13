package chat

import (
	"context"
	"fmt"
	"io"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
)

func (u *Usecase) attachmentsDir(ctx context.Context, chatID string) (string, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: attachments: chat: %w", err)
	}
	home, err := u.home()
	if err != nil {
		return "", fmt.Errorf("agent: attachments: home: %w", err)
	}
	return worktreepath.AttachmentsDir(home, chat.ID), nil
}

// BeginAttachmentUpload starts, or resumes, a chunked upload of fileName and answers the bytes the
// daemon already holds for id.
func (u *Usecase) BeginAttachmentUpload(ctx context.Context, chatID, id, fileName string, size int64) (int64, error) {
	dir, err := u.attachmentsDir(ctx, chatID)
	if err != nil {
		return 0, err
	}
	return repoattachments.BeginUpload(dir, id, fileName, size)
}

// AppendAttachmentUpload writes one chunk at offset and answers the bytes held afterwards.
func (u *Usecase) AppendAttachmentUpload(ctx context.Context, chatID, id string, offset int64, chunk io.Reader) (int64, error) {
	dir, err := u.attachmentsDir(ctx, chatID)
	if err != nil {
		return 0, err
	}
	return repoattachments.AppendUpload(dir, id, offset, chunk)
}

// CompleteAttachmentUpload stores the staged bytes as the attachment, the same record UploadAttachment makes.
func (u *Usecase) CompleteAttachmentUpload(ctx context.Context, chatID, id, fileName string, size int64) (StoredAttachment, error) {
	dir, err := u.attachmentsDir(ctx, chatID)
	if err != nil {
		return StoredAttachment{}, err
	}
	stored, contentType, err := repoattachments.CompleteUpload(dir, id, fileName, size)
	if err != nil {
		return StoredAttachment{}, err
	}
	return storedAttachment(chatID, stored, contentType, int(size)), nil
}

// AbortAttachmentUpload deletes a chunked upload's staged bytes.
func (u *Usecase) AbortAttachmentUpload(ctx context.Context, chatID, id string) error {
	dir, err := u.attachmentsDir(ctx, chatID)
	if err != nil {
		return err
	}
	return repoattachments.AbortUpload(dir, id)
}
