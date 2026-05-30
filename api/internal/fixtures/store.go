package fixtures

import "sync"

type Store struct {
	// Read-only after Load() — no lock needed for reads
	Flows         []FlowDefinition
	FileTree      FileNode
	GitLog        []Commit
	GitBranches   []Branch
	GitStatus     GitStatus
	Conversations map[string][]ChatMessage

	mu         sync.RWMutex
	workspaces map[string]WorkspacePayload
	projects   []Project
}

func NewStore() *Store {
	return &Store{
		workspaces:    make(map[string]WorkspacePayload),
		Conversations: make(map[string][]ChatMessage),
	}
}

func (s *Store) GetWorkspace(id string) (WorkspacePayload, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ws, ok := s.workspaces[id]
	return ws, ok
}

func (s *Store) AddWorkspace(ws WorkspacePayload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces[ws.ID] = ws
}

func (s *Store) ListWorkspaces() []WorkspacePayload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]WorkspacePayload, 0, len(s.workspaces))
	for _, ws := range s.workspaces {
		out = append(out, ws)
	}
	return out
}

func (s *Store) ListProjects() []Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Project, len(s.projects))
	copy(out, s.projects)
	return out
}

func (s *Store) AddProject(p Project) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects = append(s.projects, p)
}
