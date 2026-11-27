package dto

type ChatAttachmentDTO struct {
	Ref         string `json:"ref"`
	FileName    string `json:"fileName"`
	Size        int    `json:"size"`
	ContentType string `json:"contentType"`
}
