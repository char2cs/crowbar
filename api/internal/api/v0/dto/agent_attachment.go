package dto

// ChatAttachmentDTO is the upload endpoint's response: the durable logical
// reference the client encodes into the message's own markdown text, plus
// enough metadata to render a file card without a second fetch.
type ChatAttachmentDTO struct {
	Ref         string `json:"ref"`
	FileName    string `json:"fileName"`
	Size        int    `json:"size"`
	ContentType string `json:"contentType"`
}
