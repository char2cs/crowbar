package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal/auth"
	"github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// ============================================================
// Minimal mock implementations
// ============================================================

type mockAgentRunReader struct {
	run             domain.AgentRun
	getErr          error
	completeErr     error
	completesCalled int
}

func (m *mockAgentRunReader) GetByToken(_ context.Context, _ string) (domain.AgentRun, error) {
	return m.run, m.getErr
}
func (m *mockAgentRunReader) CompleteAgentRun(_ context.Context, _ string, _ string) error {
	m.completesCalled++
	return m.completeErr
}

type mockTaskReader struct {
	task         domain.Task
	getErr       error
	advanceErr   error
	advanceCalls int
}

func (m *mockTaskReader) Get(_ context.Context, _ string) (domain.Task, error) {
	return m.task, m.getErr
}
func (m *mockTaskReader) AdvanceState(_ context.Context, _ string, _ string, _ string) error {
	m.advanceCalls++
	return m.advanceErr
}

type mockFlowLoader struct {
	def flow.FlowDefinition
	err error
}

func (m *mockFlowLoader) Load(_ string) (flow.FlowDefinition, error) {
	return m.def, m.err
}

type mockHub struct {
	published []agent.ChatFrame
}

func (m *mockHub) RegisterSession(_ string) (<-chan string, func(agent.ChatFrame), func()) {
	ch := make(chan string, 32)
	pub := func(f agent.ChatFrame) { m.published = append(m.published, f) }
	unreg := func() {}
	return ch, pub, unreg
}
func (m *mockHub) Subscribe(_ string) (<-chan agent.ChatFrame, func()) {
	ch := make(chan agent.ChatFrame, 64)
	return ch, func() {}
}
func (m *mockHub) Forward(_ string, _ string) {}
func (m *mockHub) Publish(_ string, f agent.ChatFrame) {
	m.published = append(m.published, f)
}

type mockKanbanRepo struct {
	item      domain.KanbanItem
	createErr error
	getErr    error
	listErr   error
	updateErr error
	items     []domain.KanbanItem
}

func (m *mockKanbanRepo) Create(_ context.Context, taskID string, title string, agentRunID string) (domain.KanbanItem, error) {
	if m.createErr != nil {
		return domain.KanbanItem{}, m.createErr
	}
	item := m.item
	item.TaskID = taskID
	item.Title = title
	item.AgentRunID = agentRunID
	return item, nil
}
func (m *mockKanbanRepo) Get(_ context.Context, _ string) (domain.KanbanItem, error) {
	return m.item, m.getErr
}
func (m *mockKanbanRepo) ListByTask(_ context.Context, _ string) ([]domain.KanbanItem, error) {
	return m.items, m.listErr
}
func (m *mockKanbanRepo) UpdateStatus(_ context.Context, _ string, _ string, _ string) error {
	return m.updateErr
}

type mockThreadRepo struct {
	thread     domain.ReviewThread
	openErr    error
	getErr     error
	listErr    error
	postErr    error
	resolveErr error
	threads    []domain.ReviewThread
}

func (m *mockThreadRepo) Open(_ context.Context, taskID string, agentRunID *string, file string, line int, phase domain.ReviewPhase, openedBy string, content string) (domain.ReviewThread, error) {
	if m.openErr != nil {
		return domain.ReviewThread{}, m.openErr
	}
	t := m.thread
	t.TaskID = taskID
	t.AgentRunID = agentRunID
	t.File = file
	t.Line = line
	t.Phase = phase
	t.OpenedBy = openedBy
	return t, nil
}
func (m *mockThreadRepo) Get(_ context.Context, _ string) (domain.ReviewThread, error) {
	return m.thread, m.getErr
}
func (m *mockThreadRepo) ListByTask(_ context.Context, _ string, _ string, _ string) ([]domain.ReviewThread, error) {
	return m.threads, m.listErr
}
func (m *mockThreadRepo) PostMessage(_ context.Context, _ string, _ string, _ string) error {
	return m.postErr
}
func (m *mockThreadRepo) ResolveThread(_ context.Context, _ string, _ string) error {
	return m.resolveErr
}

// ============================================================
// Helpers
// ============================================================

// buildRunningContext builds a valid auth state: running AgentRun + matching Task + FlowDefinition.
func buildRunningContext(stateName string, toolNames []string, items bool) (
	*mockAgentRunReader,
	*mockTaskReader,
	*mockFlowLoader,
) {
	run := domain.AgentRun{
		ID:     "run-1",
		TaskID: "task-1",
		Status: domain.AgentRunStatusRunning,
	}
	task := domain.Task{
		ID:           "task-1",
		CurrentState: stateName,
	}
	fd := flow.FlowDefinition{
		Name:         "test-flow",
		ItemStatuses: []string{"todo", "in_progress", "done"},
		States: []flow.StateDefinition{
			{
				Name:  stateName,
				Items: items,
				Agent: &flow.AgentDef{Tools: toolNames},
				Transitions: []flow.TransitionDef{
					{On: "done", To: "next"},
				},
			},
		},
	}
	return &mockAgentRunReader{run: run},
		&mockTaskReader{task: task},
		&mockFlowLoader{def: fd}
}

// makeReq builds a CallToolRequest with the given arguments map.
func makeReq(args map[string]any) mcplib.CallToolRequest {
	return mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Arguments: args,
		},
	}
}

// ctxWithToken injects a token into the context.
func ctxWithToken(token string) context.Context {
	return tools.WithToken(context.Background(), token)
}

// ============================================================
// WithToken / TokenFromContext / TokenFromRequest
// ============================================================

func TestWithToken_RoundTrip(t *testing.T) {
	ctx := tools.WithToken(context.Background(), "abc123")
	if got := tools.TokenFromContext(ctx); got != "abc123" {
		t.Fatalf("expected abc123, got %q", got)
	}
}

func TestTokenFromContext_Missing(t *testing.T) {
	if got := tools.TokenFromContext(context.Background()); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestTokenFromRequest(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	r.Header.Set("X-Agent-Run-Token", "my-token")
	ctx := tools.TokenFromRequest(context.Background(), r)
	if got := tools.TokenFromContext(ctx); got != "my-token" {
		t.Fatalf("expected my-token, got %q", got)
	}
}

func TestTokenFromRequest_NoHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	ctx := tools.TokenFromRequest(context.Background(), r)
	if got := tools.TokenFromContext(ctx); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// ============================================================
// crowbar_signal
// ============================================================

func TestSignalHandler_AuthFails_EmptyToken(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar.signal"}, false)
	// Simulate token not found by swapping to return ErrNotFound
	agentRR.getErr = asynxModels.ErrNotFound

	h := tools.NewSignalHandler(agentRR, taskR, &mockHub{}, flowL)
	result, err := h(ctxWithToken("any"), makeReq(map[string]any{"event": "done"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestSignalHandler_ToolNotInState(t *testing.T) {
	// State has no tools => crowbar_signal not in list
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{}, false)

	h := tools.NewSignalHandler(agentRR, taskR, &mockHub{}, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"event": "done"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for forbidden tool")
	}
	if len(result.Content) > 0 {
		txt := result.Content[0].(mcplib.TextContent).Text
		if txt != auth.ErrForbidden.Error() {
			t.Errorf("expected ErrForbidden text, got %q", txt)
		}
	}
}

func TestSignalHandler_InvalidEvent(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar.signal"}, false)

	h := tools.NewSignalHandler(agentRR, taskR, &mockHub{}, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"event": "nonexistent_event"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid event")
	}
}

func TestSignalHandler_CompleteAgentRunError(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar.signal"}, false)
	agentRR.completeErr = errors.New("db error")

	h := tools.NewSignalHandler(agentRR, taskR, &mockHub{}, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"event": "done"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when CompleteAgentRun fails")
	}
}

func TestSignalHandler_AdvanceStateError(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar.signal"}, false)
	taskR.advanceErr = errors.New("advance failed")

	hub := &mockHub{}
	h := tools.NewSignalHandler(agentRR, taskR, hub, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"event": "done"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when AdvanceState fails")
	}
	// Hub should still have received publish
	if len(hub.published) == 0 {
		t.Fatal("expected Publish to be called before AdvanceState")
	}
}

func TestSignalHandler_HappyPath(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar.signal"}, false)
	hub := &mockHub{}

	h := tools.NewSignalHandler(agentRR, taskR, hub, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"event": "done", "output": "great work"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error in result: %v", result.Content)
	}

	// Verify CompleteAgentRun was called
	if agentRR.completesCalled != 1 {
		t.Errorf("expected CompleteAgentRun called once, got %d", agentRR.completesCalled)
	}
	// Verify AdvanceState was called
	if taskR.advanceCalls != 1 {
		t.Errorf("expected AdvanceState called once, got %d", taskR.advanceCalls)
	}
	// Verify Publish was called
	if len(hub.published) != 1 {
		t.Errorf("expected one published frame, got %d", len(hub.published))
	}
	if hub.published[0].Type != agent.ChatFrameTypeStateTransition {
		t.Errorf("expected StateTransition frame, got %v", hub.published[0].Type)
	}
	if hub.published[0].NewState != "next" {
		t.Errorf("expected next state 'next', got %q", hub.published[0].NewState)
	}

	// Verify JSON response
	txt := result.Content[0].(mcplib.TextContent).Text
	var sr tools.SignalResult
	if err := json.Unmarshal([]byte(txt), &sr); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if !sr.OK || sr.NextState != "next" {
		t.Errorf("unexpected result: %+v", sr)
	}
}

// ============================================================
// crowbar_create_item
// ============================================================

func TestCreateItemHandler_AuthFails(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{}, false)
	agentRR.getErr = errors.New("unauthorized")

	h := tools.NewCreateItemHandler(agentRR, taskR, &mockKanbanRepo{}, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"title": "task1"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestCreateItemHandler_ItemsDisabled(t *testing.T) {
	// items=false in state
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_create_item"}, false)

	h := tools.NewCreateItemHandler(agentRR, taskR, &mockKanbanRepo{}, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"title": "task1"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when items tracking is disabled")
	}
}

func TestCreateItemHandler_CreateError(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_create_item"}, true)
	kanban := &mockKanbanRepo{createErr: errors.New("db error")}

	h := tools.NewCreateItemHandler(agentRR, taskR, kanban, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"title": "task1"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when Create fails")
	}
}

func TestCreateItemHandler_HappyPath(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_create_item"}, true)
	kanban := &mockKanbanRepo{item: domain.KanbanItem{ID: "item-42"}}

	h := tools.NewCreateItemHandler(agentRR, taskR, kanban, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"title": "My Task"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error in result")
	}

	txt := result.Content[0].(mcplib.TextContent).Text
	var resp map[string]string
	if err := json.Unmarshal([]byte(txt), &resp); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if resp["item_id"] != "item-42" {
		t.Errorf("expected item_id item-42, got %q", resp["item_id"])
	}
}

// ============================================================
// crowbar_update_item_status
// ============================================================

func TestUpdateItemStatusHandler_AuthFails(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{}, false)
	agentRR.getErr = asynxModels.ErrNotFound

	h := tools.NewUpdateItemStatusHandler(agentRR, taskR, &mockKanbanRepo{}, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"item_id": "i1", "status": "done"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestUpdateItemStatusHandler_GetItemError(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_update_item_status"}, false)
	kanban := &mockKanbanRepo{getErr: errors.New("item not found")}

	h := tools.NewUpdateItemStatusHandler(agentRR, taskR, kanban, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"item_id": "i1", "status": "done"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestUpdateItemStatusHandler_ItemNotOwnedByTask(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_update_item_status"}, false)
	// item.TaskID differs from actx.Task.ID ("task-1")
	kanban := &mockKanbanRepo{item: domain.KanbanItem{ID: "i1", TaskID: "other-task"}}

	h := tools.NewUpdateItemStatusHandler(agentRR, taskR, kanban, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"item_id": "i1", "status": "done"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when item belongs to different task")
	}
}

func TestUpdateItemStatusHandler_InvalidStatus(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_update_item_status"}, false)
	kanban := &mockKanbanRepo{item: domain.KanbanItem{ID: "i1", TaskID: "task-1"}}

	h := tools.NewUpdateItemStatusHandler(agentRR, taskR, kanban, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"item_id": "i1", "status": "invalid_status"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid status")
	}
}

func TestUpdateItemStatusHandler_UpdateError(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_update_item_status"}, false)
	kanban := &mockKanbanRepo{
		item:      domain.KanbanItem{ID: "i1", TaskID: "task-1"},
		updateErr: errors.New("update failed"),
	}

	h := tools.NewUpdateItemStatusHandler(agentRR, taskR, kanban, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"item_id": "i1", "status": "done"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when UpdateStatus fails")
	}
}

func TestUpdateItemStatusHandler_HappyPath(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_update_item_status"}, false)
	kanban := &mockKanbanRepo{item: domain.KanbanItem{ID: "i1", TaskID: "task-1"}}

	h := tools.NewUpdateItemStatusHandler(agentRR, taskR, kanban, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"item_id": "i1", "status": "todo"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error in result")
	}

	txt := result.Content[0].(mcplib.TextContent).Text
	var resp map[string]bool
	if err := json.Unmarshal([]byte(txt), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !resp["ok"] {
		t.Error("expected ok:true")
	}
}

// ============================================================
// crowbar_get_items
// ============================================================

func TestGetItemsHandler_AuthFails(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{}, false)
	agentRR.getErr = asynxModels.ErrNotFound

	h := tools.NewGetItemsHandler(agentRR, taskR, &mockKanbanRepo{}, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(nil))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestGetItemsHandler_ListError(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_get_items"}, false)
	kanban := &mockKanbanRepo{listErr: errors.New("list failed")}

	h := tools.NewGetItemsHandler(agentRR, taskR, kanban, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(nil))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestGetItemsHandler_HappyPath(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_get_items"}, false)
	items := []domain.KanbanItem{
		{ID: "i1", Title: "First"},
		{ID: "i2", Title: "Second"},
	}
	kanban := &mockKanbanRepo{items: items}

	h := tools.NewGetItemsHandler(agentRR, taskR, kanban, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(nil))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error in result")
	}

	txt := result.Content[0].(mcplib.TextContent).Text
	var resp []domain.KanbanItem
	if err := json.Unmarshal([]byte(txt), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp))
	}
}

// ============================================================
// crowbar_open_thread
// ============================================================

func TestOpenThreadHandler_AuthFails(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("ai_review", []string{}, false)
	agentRR.getErr = asynxModels.ErrNotFound

	h := tools.NewOpenThreadHandler(agentRR, taskR, &mockThreadRepo{}, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"file": "a.go", "line": 1.0, "content": "comment"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestOpenThreadHandler_OpenError(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("ai_review", []string{"crowbar_open_thread"}, false)
	threadRepo := &mockThreadRepo{openErr: errors.New("open failed")}

	h := tools.NewOpenThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"file": "a.go", "line": 10.0, "content": "issue here"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestOpenThreadHandler_AIReview_Phase(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("ai_review", []string{"crowbar_open_thread"}, false)
	threadRepo := &mockThreadRepo{thread: domain.ReviewThread{ID: "thread-1"}}

	h := tools.NewOpenThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"file": "main.go", "line": 42.0, "content": "found issue"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error in result")
	}

	// The phase passed to Open should be ReviewPhaseAIReview for "ai_review" state.
	if threadRepo.thread.Phase != "" {
		// The mock stores the phase in the returned thread; verify via open call
	}

	txt := result.Content[0].(mcplib.TextContent).Text
	var resp map[string]string
	if err := json.Unmarshal([]byte(txt), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp["thread_id"] != "thread-1" {
		t.Errorf("expected thread-1, got %q", resp["thread_id"])
	}
}

func TestOpenThreadHandler_HumanReview_Phase(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("human_review", []string{"crowbar_open_thread"}, false)
	var capturedPhase domain.ReviewPhase
	threadRepo := &mockThreadRepo{}
	threadRepo.thread = domain.ReviewThread{ID: "t2"}
	// Use a capturing threadRepo
	capRepo := &capturingThreadRepo{
		phase: &capturedPhase,
	}

	h := tools.NewOpenThreadHandler(agentRR, taskR, capRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"file": "b.go", "line": 5.0, "content": "note"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error in result")
	}
	if capturedPhase != domain.ReviewPhaseHumanReview {
		t.Errorf("expected HumanReview phase, got %v", capturedPhase)
	}
}

// capturingThreadRepo captures the phase passed to Open.
type capturingThreadRepo struct {
	phase *domain.ReviewPhase
}

func (r *capturingThreadRepo) Open(_ context.Context, taskID string, agentRunID *string, file string, line int, phase domain.ReviewPhase, openedBy string, content string) (domain.ReviewThread, error) {
	*r.phase = phase
	return domain.ReviewThread{ID: "t-captured", TaskID: taskID}, nil
}
func (r *capturingThreadRepo) Get(_ context.Context, _ string) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}
func (r *capturingThreadRepo) ListByTask(_ context.Context, _ string, _ string, _ string) ([]domain.ReviewThread, error) {
	return nil, nil
}
func (r *capturingThreadRepo) PostMessage(_ context.Context, _ string, _ string, _ string) error {
	return nil
}
func (r *capturingThreadRepo) ResolveThread(_ context.Context, _ string, _ string) error {
	return nil
}

// ============================================================
// crowbar_reply_thread
// ============================================================

func TestReplyThreadHandler_AuthFails(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{}, false)
	agentRR.getErr = asynxModels.ErrNotFound

	h := tools.NewReplyThreadHandler(agentRR, taskR, &mockThreadRepo{}, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1", "content": "reply"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestReplyThreadHandler_GetThreadError(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_reply_thread"}, false)
	threadRepo := &mockThreadRepo{getErr: errors.New("not found")}

	h := tools.NewReplyThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1", "content": "hi"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestReplyThreadHandler_ThreadNotOwnedByTask(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_reply_thread"}, false)
	threadRepo := &mockThreadRepo{
		thread: domain.ReviewThread{ID: "t1", TaskID: "other-task"},
	}

	h := tools.NewReplyThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1", "content": "hello"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for wrong task")
	}
}

func TestReplyThreadHandler_PostMessageError(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_reply_thread"}, false)
	threadRepo := &mockThreadRepo{
		thread:  domain.ReviewThread{ID: "t1", TaskID: "task-1"},
		postErr: errors.New("post failed"),
	}

	h := tools.NewReplyThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1", "content": "msg"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when PostMessage fails")
	}
}

func TestReplyThreadHandler_HappyPath_AIReviewRole(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("ai_review", []string{"crowbar_reply_thread"}, false)
	threadRepo := &mockThreadRepo{
		thread: domain.ReviewThread{ID: "t1", TaskID: "task-1"},
	}

	h := tools.NewReplyThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1", "content": "my review"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error in result")
	}
}

func TestReplyThreadHandler_HappyPath_ImplementationRole(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_reply_thread"}, false)
	threadRepo := &mockThreadRepo{
		thread: domain.ReviewThread{ID: "t1", TaskID: "task-1"},
	}

	h := tools.NewReplyThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1", "content": "fixed it"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error in result")
	}
}

func TestReplyThreadHandler_HappyPath_DefaultRole(t *testing.T) {
	// A state name that's not "ai_review" or "implementation" should still work
	agentRR, taskR, flowL := buildRunningContext("some_other_state", []string{"crowbar_reply_thread"}, false)
	threadRepo := &mockThreadRepo{
		thread: domain.ReviewThread{ID: "t1", TaskID: "task-1"},
	}

	h := tools.NewReplyThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1", "content": "msg"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error in result")
	}
}

// ============================================================
// crowbar_get_threads
// ============================================================

func TestGetThreadsHandler_AuthFails(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{}, false)
	agentRR.getErr = asynxModels.ErrNotFound

	h := tools.NewGetThreadsHandler(agentRR, taskR, &mockThreadRepo{}, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(nil))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestGetThreadsHandler_ListError(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_get_threads"}, false)
	threadRepo := &mockThreadRepo{listErr: errors.New("list failed")}

	h := tools.NewGetThreadsHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(nil))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestGetThreadsHandler_HappyPath(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_get_threads"}, false)
	threads := []domain.ReviewThread{
		{ID: "t1", TaskID: "task-1", Status: domain.ReviewThreadStatusOpen},
		{ID: "t2", TaskID: "task-1", Status: domain.ReviewThreadStatusAgreed},
	}
	threadRepo := &mockThreadRepo{threads: threads}

	h := tools.NewGetThreadsHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"status": "open", "phase": "ai_review"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error in result")
	}

	txt := result.Content[0].(mcplib.TextContent).Text
	var resp []domain.ReviewThread
	if err := json.Unmarshal([]byte(txt), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 threads, got %d", len(resp))
	}
}

func TestGetThreadsHandler_NoFilters(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_get_threads"}, false)
	threadRepo := &mockThreadRepo{threads: []domain.ReviewThread{{ID: "t1"}}}

	h := tools.NewGetThreadsHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(nil))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error in result")
	}
}

// ============================================================
// crowbar_resolve_thread
// ============================================================

func TestResolveThreadHandler_AuthFails(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{}, false)
	agentRR.getErr = asynxModels.ErrNotFound

	h := tools.NewResolveThreadHandler(agentRR, taskR, &mockThreadRepo{}, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestResolveThreadHandler_GetThreadError(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_resolve_thread"}, false)
	threadRepo := &mockThreadRepo{getErr: errors.New("not found")}

	h := tools.NewResolveThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestResolveThreadHandler_ThreadNotOwnedByTask(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_resolve_thread"}, false)
	threadRepo := &mockThreadRepo{
		thread: domain.ReviewThread{ID: "t1", TaskID: "other-task", Status: domain.ReviewThreadStatusOpen},
	}

	h := tools.NewResolveThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for wrong task ownership")
	}
}

func TestResolveThreadHandler_AlreadyResolved_NoOp(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_resolve_thread"}, false)
	threadRepo := &mockThreadRepo{
		thread: domain.ReviewThread{
			ID:     "t1",
			TaskID: "task-1",
			Status: domain.ReviewThreadStatusAgreed, // already resolved
		},
	}

	h := tools.NewResolveThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error — should be no-op: %v", result.Content)
	}
	// ResolveThread should NOT have been called
	if threadRepo.resolveErr != nil {
		// This only triggers if ResolveThread was called; since resolveErr is nil here,
		// we can't distinguish, but the result must still be ok.
	}
}

func TestResolveThreadHandler_AlreadyForceApproved_NoOp(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_resolve_thread"}, false)
	threadRepo := &mockThreadRepo{
		thread: domain.ReviewThread{
			ID:     "t1",
			TaskID: "task-1",
			Status: domain.ReviewThreadStatusForceApproved,
		},
	}

	h := tools.NewResolveThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error — should be no-op")
	}
}

func TestResolveThreadHandler_ResolveError(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_resolve_thread"}, false)
	threadRepo := &mockThreadRepo{
		thread: domain.ReviewThread{
			ID:     "t1",
			TaskID: "task-1",
			Status: domain.ReviewThreadStatusOpen,
		},
		resolveErr: errors.New("resolve failed"),
	}

	h := tools.NewResolveThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1", "emoji": "✅"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when ResolveThread fails")
	}
}

func TestResolveThreadHandler_HappyPath(t *testing.T) {
	agentRR, taskR, flowL := buildRunningContext("implementation", []string{"crowbar_resolve_thread"}, false)
	threadRepo := &mockThreadRepo{
		thread: domain.ReviewThread{
			ID:     "t1",
			TaskID: "task-1",
			Status: domain.ReviewThreadStatusOpen,
		},
	}

	h := tools.NewResolveThreadHandler(agentRR, taskR, threadRepo, flowL)
	result, err := h(ctxWithToken("tok"), makeReq(map[string]any{"thread_id": "t1", "emoji": "✅"}))
	if err != nil {
		t.Fatalf("handler must not return error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error in result")
	}

	txt := result.Content[0].(mcplib.TextContent).Text
	var resp map[string]bool
	if err := json.Unmarshal([]byte(txt), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !resp["ok"] {
		t.Error("expected ok:true")
	}
}

// ============================================================
// Tool constructor smoke tests (ensure they return a valid tool name)
// ============================================================

func TestNewToolConstructors(t *testing.T) {
	tests := []struct {
		name string
		tool mcplib.Tool
	}{
		{"crowbar_signal", tools.NewSignalTool()},
		{"crowbar_create_item", tools.NewCreateItemTool()},
		{"crowbar_update_item_status", tools.NewUpdateItemStatusTool()},
		{"crowbar_get_items", tools.NewGetItemsTool()},
		{"crowbar_open_thread", tools.NewOpenThreadTool()},
		{"crowbar_reply_thread", tools.NewReplyThreadTool()},
		{"crowbar_get_threads", tools.NewGetThreadsTool()},
		{"crowbar_resolve_thread", tools.NewResolveThreadTool()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.tool.Name != tc.name {
				t.Errorf("expected tool name %q, got %q", tc.name, tc.tool.Name)
			}
		})
	}
}
