package turn

import (
	"context"
	"fmt"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

// UploadAttachmentInput is one attachment's identity and bytes, already
// extracted from whichever of the handler's two ingestion shapes the request
// used — see chat/handlers.UploadAttachment.
type UploadAttachmentInput struct {
	// ID is the client-supplied shortid (nanoid); the backend never mints one
	// (design spec: it may need to match an Excalidraw fence-tag id).
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
func (t *Turns) UploadAttachment(
	ctx context.Context,
	chatID string,
	in UploadAttachmentInput,
) (StoredAttachment, error) {
	chat, err := t.chats.GetChat(ctx, chatID)
	if err != nil {
		return StoredAttachment{}, fmt.Errorf("agent: upload attachment: chat: %w", err)
	}
	chatsDir, err := t.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return StoredAttachment{}, fmt.Errorf("agent: upload attachment: chats dir: %w", err)
	}
	dir := worktreepath.AttachmentsDir(chatsDir, chatID)
	fileName, contentType, err := repoattachments.Store(dir, in.ID, in.OriginalName, in.Data)
	if err != nil {
		return StoredAttachment{}, fmt.Errorf("agent: upload attachment: %w", err)
	}
	return StoredAttachment{
		Ref: "chats/" + chatID + "/attachments/" + fileName, FileName: fileName,
		Size: len(in.Data), ContentType: contentType,
	}, nil
}

// ReadAttachment resolves chatID's stored attachment fileName to bytes and a
// sniffed content type, or repoattachments.ErrNotFound.
func (t *Turns) ReadAttachment(
	ctx context.Context,
	chatID, fileName string,
) ([]byte, string, error) {
	chat, err := t.chats.GetChat(ctx, chatID)
	if err != nil {
		return nil, "", fmt.Errorf("agent: read attachment: chat: %w", err)
	}
	chatsDir, err := t.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return nil, "", fmt.Errorf("agent: read attachment: chats dir: %w", err)
	}
	dir := worktreepath.AttachmentsDir(chatsDir, chatID)
	return repoattachments.Read(dir, fileName)
}
