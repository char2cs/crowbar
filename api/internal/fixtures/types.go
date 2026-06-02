package fixtures

import "time"

type UIMode string

const (
	UIModeChat UIMode = "chat"
	UIModeDiff UIMode = "diff"
)

type FlowStateDefinition struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	UI    UIMode `json:"ui"`
}

type FlowDefinition struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	States      []FlowStateDefinition `json:"states"`
}

type WorkspacePayload struct {
	ID           string         `json:"id"`
	RepoID       string         `json:"repoId"`
	Branch       string         `json:"branch"`
	FlowName     string         `json:"flowName"`
	CurrentState string         `json:"currentState"`
	Flow         FlowDefinition `json:"flow"`
}

type ChatMessage struct {
	ID             string  `json:"id"`
	Role           string  `json:"role"`
	Content        string  `json:"content"`
	AuthorName     string  `json:"authorName"`
	AuthorInitials string  `json:"authorInitials"`
	ModelName      string  `json:"modelName,omitempty"`
	Timestamp      string  `json:"timestamp"`
	ToolCalls      int     `json:"toolCalls,omitempty"`
	DurationSec    float64 `json:"durationSec,omitempty"`
}

type Project struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	LastActivity time.Time `json:"lastActivity"`
}

type FileStatus string

const (
	FileStatusModified FileStatus = "modified"
	FileStatusAdded    FileStatus = "added"
	FileStatusDeleted  FileStatus = "deleted"
	FileStatusRenamed  FileStatus = "renamed"
)

type GitFile struct {
	Path   string     `json:"path"`
	Status FileStatus `json:"status"`
}

type GitStatus struct {
	Branch   string    `json:"branch"`
	Staged   []GitFile `json:"staged"`
	Unstaged []GitFile `json:"unstaged"`
}

type Commit struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"shortHash"`
	Message   string `json:"message"`
	Author    string `json:"author"`
	Date      string `json:"date"`
}

type Branch struct {
	Name       string `json:"name"`
	IsCurrent  bool   `json:"isCurrent"`
	IsRemote   bool   `json:"isRemote"`
	LastCommit string `json:"lastCommit,omitempty"`
}

type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Type     string     `json:"type"`
	Size     int64      `json:"size,omitempty"`
	Children []FileNode `json:"children,omitempty"`
}

// WS event shapes — one per channel

type WorkspaceEvent struct {
	WorkspaceID string `json:"workspaceId"`
	Action      string `json:"action"` // "created" | "updated" | "deleted"
}

type GitEvent struct {
	Repo    string `json:"repo"`
	Changed bool   `json:"changed"`
}

type FileEvent struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

type ChatChunk struct {
	ChatID  string `json:"chatId"`
	Content string `json:"content"`
	Done    bool   `json:"done"`
}

type TerminalFrame struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
	IsInput   bool   `json:"isInput"`
}

type DaemonStatus struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}

// Branch-review + markdown-chat shapes — mirror the frontend's MSW types so the
// desktop daemon can serve the same surfaces the browser mocks (diff, chats,
// threads, description, markdown turns).

type DiffLine struct {
	LineType      string `json:"line_type"`
	Content       string `json:"content"`
	OldLineNumber int    `json:"old_line_number,omitempty"`
	NewLineNumber int    `json:"new_line_number,omitempty"`
}

type FileDiff struct {
	FilePath  string     `json:"file_path"`
	IsNew     bool       `json:"is_new"`
	IsDeleted bool       `json:"is_deleted"`
	IsRenamed bool       `json:"is_renamed"`
	Additions int        `json:"additions,omitempty"`
	Deletions int        `json:"deletions,omitempty"`
	Lines     []DiffLine `json:"lines"`
}

type MultiFileDiff struct {
	CommitHash     string     `json:"commitHash"`
	CommitMessage  string     `json:"commitMessage,omitempty"`
	Files          []FileDiff `json:"files"`
	TotalFiles     int        `json:"totalFiles"`
	TotalAdditions int        `json:"totalAdditions"`
	TotalDeletions int        `json:"totalDeletions"`
}

type ReviewConversation struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Age      string `json:"age"`
	IsActive bool   `json:"isActive"`
}

type MarkdownTurn struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	Timestamp  string `json:"timestamp"`
	AuthorName string `json:"authorName"`
	Model      string `json:"model,omitempty"`
	Widgets    []any  `json:"widgets"`
}
