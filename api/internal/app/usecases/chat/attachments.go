package chat

import (
	"context"
	"fmt"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
)

type UploadAttachmentInput struct {
	ID           string
	OriginalName string
	Data         []byte
}

type StoredAttachment struct {
	Ref         string
	FileName    string
	Size        int
	ContentType string
}

func (u *Usecase) UploadAttachment(
	ctx context.Context,
	chatID string,
	in UploadAttachmentInput,
) (StoredAttachment, error) {
	dir, err := u.attachmentsDir(ctx, chatID)
	if err != nil {
		return StoredAttachment{}, err
	}
	fileName, contentType, err := repoattachments.Store(dir, in.ID, in.OriginalName, in.Data)
	if err != nil {
		return StoredAttachment{}, fmt.Errorf("agent: upload attachment: %w", err)
	}
	return storedAttachment(chatID, fileName, contentType, len(in.Data)), nil
}

func storedAttachment(chatID, fileName, contentType string, size int) StoredAttachment {
	return StoredAttachment{
		Ref: "chats/" + chatID + "/attachments/" + fileName, FileName: fileName,
		Size: size, ContentType: contentType,
	}
}

func (u *Usecase) ReadAttachment(
	ctx context.Context,
	chatID, fileName string,
) ([]byte, string, error) {
	dir, err := u.attachmentsDir(ctx, chatID)
	if err != nil {
		return nil, "", err
	}
	data, contentType, err := repoattachments.Read(dir, fileName)
	if err != nil {
		return nil, "", fmt.Errorf("agent: read attachment: %w", err)
	}
	return data, contentType, nil
}
