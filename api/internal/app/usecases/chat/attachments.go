package chat

import (
	"context"
	"fmt"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
)

// UploadAttachmentInput is one attachment's identity and bytes, already
// extracted from whichever of the handler's two ingestion shapes the request
// used — see chat/handlers.UploadAttachment.
type UploadAttachmentInput struct {
	// ID is the client-supplied shortid (nanoid); the backend never mints one
	// (it may need to match an Excalidraw fence-tag id).
	ID           string
	OriginalName string
	Data         []byte
}

// StoredAttachment is what a successful upload becomes: the durable logical
// reference the caller writes back into the message's markdown text, plus
// metadata a file card renders without a second fetch.
type StoredAttachment struct {
	Ref         string
	FileName    string
	Size        int
	ContentType string
}

// UploadAttachment stores one attachment into chatID's durable attachment
// directory and returns the logical reference the frontend encodes into
// ![]()/[]().
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

// ReadAttachment resolves chatID's stored attachment fileName to bytes and a
// sniffed content type, or repoattachments.ErrNotFound.
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
