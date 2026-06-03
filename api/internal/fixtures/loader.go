package fixtures

import (
	"encoding/json"
	_ "embed"
)

//go:embed workspaces.json
var workspacesJSON []byte

//go:embed flows.json
var flowsJSON []byte

//go:embed projects.json
var projectsJSON []byte

//go:embed conversations.json
var conversationsJSON []byte

//go:embed git-log.json
var gitLogJSON []byte

//go:embed git-branches.json
var gitBranchesJSON []byte

//go:embed git-status.json
var gitStatusJSON []byte

//go:embed file-tree.json
var fileTreeJSON []byte

// Load reads all embedded fixture files and returns a populated Store.
// Workspaces have their Flow field populated by joining on flowName.
func Load() (*Store, error) {
	s := NewStore()

	if err := json.Unmarshal(flowsJSON, &s.Flows); err != nil {
		return nil, err
	}

	flowByName := make(map[string]FlowDefinition, len(s.Flows))
	for _, f := range s.Flows {
		flowByName[f.Name] = f
	}

	var wsSlice []WorkspacePayload
	if err := json.Unmarshal(workspacesJSON, &wsSlice); err != nil {
		return nil, err
	}
	for _, ws := range wsSlice {
		ws.Flow = flowByName[ws.FlowName]
		s.workspaces[ws.ID] = ws
	}

	var projects []Project
	if err := json.Unmarshal(projectsJSON, &projects); err != nil {
		return nil, err
	}
	s.projects = projects

	if err := json.Unmarshal(conversationsJSON, &s.Conversations); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(gitLogJSON, &s.GitLog); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(gitBranchesJSON, &s.GitBranches); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(gitStatusJSON, &s.GitStatus); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(fileTreeJSON, &s.FileTree); err != nil {
		return nil, err
	}

	return s, nil
}
