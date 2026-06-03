//go:build integration || manual

package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	githandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// CreateProjectRequest is the request body for POST /api/v0/projects.
type CreateProjectRequest struct {
	Name string `json:"name"`
}

// CreateRepositoryRequest is the request body for POST /api/v0/projects/:id/repositories.
type CreateRepositoryRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// CreateTaskRequest is the request body for POST /api/v0/repositories/:id/tasks.
type CreateTaskRequest struct {
	Title      string `json:"title"`
	BranchName string `json:"branch_name"`
	BaseBranch string `json:"base_branch"`
	FlowPath   string `json:"flow_path"`
}

// OpenThreadRequest is the request body for POST /api/v0/tasks/:id/threads.
type OpenThreadRequest struct {
	File    string `json:"file_path"`
	Line    int    `json:"line_number"`
	Content string `json:"content"`
}

// FileEntry is a single entry returned by GET /api/v0/tasks/:id/files.
type FileEntry = string

// Client is a typed REST client that routes all requests over a Unix socket.
// Each method asserts a successful status code and returns the decoded domain
// struct; failures call t.Fatalf immediately.
type Client struct {
	t          *testing.T
	sockPath   string
	httpClient *http.Client
}

func newClient(
	t *testing.T,
	sockPath string,
) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
		},
	}
	return &Client{
		t:        t,
		sockPath: sockPath,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// envelope matches the shape returned by libs.WriteQueryOK and
// libs.WriteMutationOK so the client can unwrap the data field.
type envelope struct {
	Data json.RawMessage `json:"data"`
}

func (c *Client) doJSON(
	method string,
	path string,
	body any,
	wantStatus int,
	out any,
) {
	c.t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("client: marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, "http://unix"+path, bodyReader)
	if err != nil {
		c.t.Fatalf("client: new request %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.t.Fatalf("client: do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		c.t.Fatalf("client: %s %s: want status %d got %d\nbody: %s", method, path, wantStatus, resp.StatusCode, b)
	}

	if out != nil {
		var env envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			c.t.Fatalf("client: decode envelope %s %s: %v", method, path, err)
		}
		if err := json.Unmarshal(env.Data, out); err != nil {
			c.t.Fatalf("client: decode data %s %s: %v", method, path, err)
		}
	}
}

// RawDo performs an HTTP request and returns the raw response for error-path
// tests that need to inspect status codes without decoding a body.
func (c *Client) RawDo(
	method string,
	path string,
	body any,
) *http.Response {
	c.t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("client: marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, "http://unix"+path, bodyReader)
	if err != nil {
		c.t.Fatalf("client: new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.t.Fatalf("client: do: %v", err)
	}
	return resp
}

// CreateProject creates a project and returns the result.
func (c *Client) CreateProject(
	req CreateProjectRequest,
) domain.Project {
	c.t.Helper()
	var out domain.Project
	c.doJSON(http.MethodPost, "/api/v0/projects", req, 201, &out)
	return out
}

// CreateRepository creates a repository under the given project.
func (c *Client) CreateRepository(
	projectID string,
	req CreateRepositoryRequest,
) domain.Repository {
	c.t.Helper()
	var out domain.Repository
	c.doJSON(http.MethodPost, fmt.Sprintf("/api/v0/projects/%s/repositories", projectID), req, 201, &out)
	return out
}

// CreateTask creates a task on the given repository.
func (c *Client) CreateTask(
	repoID string,
	req CreateTaskRequest,
) domain.Task {
	c.t.Helper()
	var out domain.Task
	c.doJSON(http.MethodPost, fmt.Sprintf("/api/v0/repositories/%s/tasks", repoID), req, 201, &out)
	return out
}

// GetTask fetches a task by ID.
func (c *Client) GetTask(
	taskID string,
) domain.Task {
	c.t.Helper()
	var out domain.Task
	c.doJSON(http.MethodGet, fmt.Sprintf("/api/v0/tasks/%s", taskID), nil, 200, &out)
	return out
}

// ArchiveTask archives the task.
func (c *Client) ArchiveTask(
	taskID string,
) {
	c.t.Helper()
	c.doJSON(http.MethodPost, fmt.Sprintf("/api/v0/tasks/%s/archive", taskID), nil, 200, nil)
}

// PauseTask pauses the task.
func (c *Client) PauseTask(
	taskID string,
) {
	c.t.Helper()
	c.doJSON(http.MethodPost, fmt.Sprintf("/api/v0/tasks/%s/pause", taskID), nil, 200, nil)
}

// ResumeTask resumes the task.
func (c *Client) ResumeTask(
	taskID string,
) {
	c.t.Helper()
	c.doJSON(http.MethodPost, fmt.Sprintf("/api/v0/tasks/%s/resume", taskID), nil, 200, nil)
}

// RetryTask retries the task by creating a new AgentRun.
func (c *Client) RetryTask(
	taskID string,
) domain.Task {
	c.t.Helper()
	var out domain.Task
	c.doJSON(http.MethodPost, fmt.Sprintf("/api/v0/tasks/%s/retry", taskID), nil, 200, &out)
	return out
}

// ForceTransition force-transitions the task to the given state, bypassing the
// normal agent flow.
func (c *Client) ForceTransition(
	taskID string,
	toState string,
) domain.Task {
	c.t.Helper()
	var out domain.Task
	c.doJSON(http.MethodPost, fmt.Sprintf("/api/v0/tasks/%s/force-transition", taskID), map[string]string{"to_state": toState}, 200, &out)
	return out
}

// ListAgentRuns returns all agent runs for the given task.
func (c *Client) ListAgentRuns(
	taskID string,
) []domain.AgentRun {
	c.t.Helper()
	var out []domain.AgentRun
	c.doJSON(http.MethodGet, fmt.Sprintf("/api/v0/tasks/%s/agent-runs", taskID), nil, 200, &out)
	return out
}

// InterruptAgentRun interrupts a running AgentRun.
func (c *Client) InterruptAgentRun(
	agentRunID string,
) {
	c.t.Helper()
	c.doJSON(http.MethodPost, fmt.Sprintf("/api/v0/agent-runs/%s/interrupt", agentRunID), nil, 204, nil)
}

// ListKanbanItems returns all kanban items for the given task.
func (c *Client) ListKanbanItems(
	taskID string,
) []domain.KanbanItem {
	c.t.Helper()
	var out []domain.KanbanItem
	c.doJSON(http.MethodGet, fmt.Sprintf("/api/v0/tasks/%s/kanban-items", taskID), nil, 200, &out)
	return out
}

// ListThreads returns review threads for the given task, optionally filtered by
// status and phase.
func (c *Client) ListThreads(
	taskID string,
	status string,
	phase string,
) []domain.ReviewThread {
	c.t.Helper()
	path := fmt.Sprintf("/api/v0/tasks/%s/threads", taskID)
	if status != "" || phase != "" {
		path += "?"
		if status != "" {
			path += "status=" + status
		}
		if phase != "" {
			if status != "" {
				path += "&"
			}
			path += "phase=" + phase
		}
	}
	var out []domain.ReviewThread
	c.doJSON(http.MethodGet, path, nil, 200, &out)
	return out
}

// OpenThread opens a review thread as a human reviewer.
func (c *Client) OpenThread(
	taskID string,
	req OpenThreadRequest,
) domain.ReviewThread {
	c.t.Helper()
	var out domain.ReviewThread
	c.doJSON(http.MethodPost, fmt.Sprintf("/api/v0/tasks/%s/threads", taskID), req, 201, &out)
	return out
}

// PostMessage posts a human reply to an existing review thread.
func (c *Client) PostMessage(
	threadID string,
	content string,
) {
	c.t.Helper()
	c.doJSON(http.MethodPost, fmt.Sprintf("/api/v0/threads/%s/messages", threadID), map[string]string{"content": content}, 204, nil)
}

// ForceApproveThread force-approves a review thread.
func (c *Client) ForceApproveThread(
	threadID string,
) domain.ReviewThread {
	c.t.Helper()
	var out domain.ReviewThread
	c.doJSON(http.MethodPost, fmt.Sprintf("/api/v0/threads/%s/force-approve", threadID), nil, 200, &out)
	return out
}

// GetGitLog returns the git commit log for the task's branch.
func (c *Client) GetGitLog(
	taskID string,
) []domain.GitCommit {
	c.t.Helper()
	var out []domain.GitCommit
	c.doJSON(http.MethodGet, fmt.Sprintf("/api/v0/tasks/%s/git/log", taskID), nil, 200, &out)
	return out
}

// GetGitDiff returns the structured git diff for the task's branch.
func (c *Client) GetGitDiff(
	taskID string,
) githandlers.GitDiff {
	c.t.Helper()
	var out githandlers.GitDiff
	c.doJSON(http.MethodGet, fmt.Sprintf("/api/v0/tasks/%s/git/diff", taskID), nil, 200, &out)
	return out
}

// ListFiles returns the list of files tracked by git at the given path in the
// task's worktree.
func (c *Client) ListFiles(
	taskID string,
	path string,
) []FileEntry {
	c.t.Helper()
	p := fmt.Sprintf("/api/v0/tasks/%s/files", taskID)
	if path != "" {
		p += "?path=" + path
	}
	var out []FileEntry
	c.doJSON(http.MethodGet, p, nil, 200, &out)
	return out
}

// GetMessages returns the full conversation history for the given task.
func (c *Client) GetMessages(
	taskID string,
) []domain.ConversationMessage {
	c.t.Helper()
	var out []domain.ConversationMessage
	c.doJSON(http.MethodGet, fmt.Sprintf("/api/v0/tasks/%s/messages", taskID), nil, 200, &out)
	return out
}
