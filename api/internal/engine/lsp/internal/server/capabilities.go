package server

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/semtok"
)

func clientCapabilities() map[string]any {
	markup := []string{"markdown", "plaintext"}
	return map[string]any{
		"textDocument": map[string]any{
			"synchronization":    map[string]any{"didSave": true, "dynamicRegistration": false},
			"publishDiagnostics": map[string]any{"relatedInformation": true},
			"hover":              map[string]any{"contentFormat": markup},
			"completion": map[string]any{
				"contextSupport": true,
				"completionItem": map[string]any{
					"snippetSupport":          true,
					"documentationFormat":     markup,
					"deprecatedSupport":       true,
					"labelDetailsSupport":     true,
					"insertReplaceSupport":    false,
					"commitCharactersSupport": false,
				},
			},
			"signatureHelp": map[string]any{
				"contextSupport": true,
				"signatureInformation": map[string]any{
					"documentationFormat":    markup,
					"activeParameterSupport": true,
					"parameterInformation":   map[string]any{"labelOffsetSupport": true},
				},
			},
			"definition":     map[string]any{"linkSupport": false},
			"references":     map[string]any{},
			"documentSymbol": map[string]any{"hierarchicalDocumentSymbolSupport": true},
			"codeAction": map[string]any{
				"isPreferredSupport": true,
				"codeActionLiteralSupport": map[string]any{
					"codeActionKind": map[string]any{
						"valueSet": []string{
							"", "quickfix", "refactor", "refactor.extract", "refactor.inline",
							"refactor.rewrite", "source", "source.organizeImports", "source.fixAll",
						},
					},
				},
			},
			"codeLens":       map[string]any{},
			"formatting":     map[string]any{},
			"rename":         map[string]any{"prepareSupport": false},
			"semanticTokens": semanticTokensCapability(),
		},
		"workspace": map[string]any{
			"workspaceFolders": true,
			"workspaceEdit":    map[string]any{"documentChanges": false},
			"applyEdit":        true,
			"executeCommand":   map[string]any{"dynamicRegistration": false},
		},
	}
}

func semanticTokensCapability() map[string]any {
	return map[string]any{
		"dynamicRegistration":     false,
		"requests":                map[string]any{"range": true, "full": map[string]any{"delta": true}},
		"tokenTypes":              semtok.TokenTypes,
		"tokenModifiers":          semtok.TokenModifiers,
		"formats":                 []string{"relative"},
		"overlappingTokenSupport": false,
		"multilineTokenSupport":   false,
		"augmentsSyntaxTokens":    true,
	}
}

type serverFeatures struct {
	semTok   semtok.Support
	commands map[string]bool
}

func featuresFromInitialize(
	result json.RawMessage,
) (serverFeatures, error) {
	var res struct {
		Capabilities json.RawMessage `json:"capabilities"`
	}
	if err := json.Unmarshal(result, &res); err != nil {
		return serverFeatures{}, fmt.Errorf("initialize result: %w", err)
	}
	var caps struct {
		ExecuteCommand struct {
			Commands []string `json:"commands"`
		} `json:"executeCommandProvider"`
	}
	if len(res.Capabilities) > 0 {
		if err := json.Unmarshal(res.Capabilities, &caps); err != nil {
			slog.Warn("lsp: executeCommandProvider unreadable; offering no commands", "err", err)
			caps.ExecuteCommand.Commands = nil
		}
	}
	commands := make(map[string]bool, len(caps.ExecuteCommand.Commands))
	for _, c := range caps.ExecuteCommand.Commands {
		commands[c] = true
	}
	return serverFeatures{semTok: semtok.FromCapabilities(res.Capabilities), commands: commands}, nil
}

func (s *server) recordFeatures(
	result json.RawMessage,
) error {
	features, err := featuresFromInitialize(result)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.features = features
	s.mu.Unlock()
	return nil
}
