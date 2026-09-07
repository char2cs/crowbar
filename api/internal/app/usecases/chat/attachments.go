package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
)

type (
	UploadAttachmentInput = turn.UploadAttachmentInput
	StoredAttachment      = turn.StoredAttachment
)

// UploadAttachment stores one attachment into chatID's durable attachment
// store and returns the logical reference to write back into the message.
func (u *Usecase) UploadAttachment(
	ctx context.Context,
	chatID string,
	in UploadAttachmentInput,
) (StoredAttachment, error) {
	return u.turns.UploadAttachment(ctx, chatID, in)
}

// ReadAttachment resolves chatID's stored attachment fileName to bytes and a
// sniffed content type.
func (u *Usecase) ReadAttachment(
	ctx context.Context,
	chatID, fileName string,
) ([]byte, string, error) {
	return u.turns.ReadAttachment(ctx, chatID, fileName)
}
