package registry

import "strings"

// AgentEntry maps a model-name prefix to an agent binary and its fixed args.
//
// ModelArg controls whether the resolved model name is appended to Args as a
// trailing CLI argument. Some agent bridges (e.g. claude-code-acp) take no CLI
// flags and select the model over the ACP protocol instead; those set ModelArg
// to false.
type AgentEntry struct {
	Bin      string
	Args     []string
	ModelArg bool
}

var entries = map[string]AgentEntry{
	// claude-code-acp is the ACP bridge for Claude Code. It accepts no CLI
	// flags; the model is chosen via the ACP session/set_model request.
	"claude-": {Bin: "claude-code-acp", Args: nil, ModelArg: false},
	"codex-":  {Bin: "codex", Args: []string{"acp", "--model"}, ModelArg: true},
	"gemini-": {Bin: "gemini", Args: []string{"--acp", "--model"}, ModelArg: true},
}

// Lookup returns the first entry whose key is a prefix of modelName.
func Lookup(
	modelName string,
) (AgentEntry, bool) {
	for prefix, entry := range entries {
		if strings.HasPrefix(modelName, prefix) {
			return entry, true
		}
	}
	return AgentEntry{}, false
}
