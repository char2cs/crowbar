package server

import (
	"encoding/json"

	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/semtok"
)

// clientCapabilities declares what the editor (Monaco, via the daemon's /lsp
// routes) can consume. Servers shape their answers by it: without
// codeActionLiteralSupport a server returns bare commands instead of edits,
// without hierarchicalDocumentSymbolSupport a flat symbol list, without a
// markdown contentFormat plain-text hovers.
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

// semanticTokensCapability advertises the canonical legend (semtok) so servers
// that shape tokens by the client's list use names the editor styles.
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

// serverFeatures is what the client reads out of a server's initialize
// result: its semantic-token support and the commands it executes.
type serverFeatures struct {
	semTok   semtok.Support
	commands map[string]bool
}

func featuresFromInitialize(
	result json.RawMessage,
) serverFeatures {
	var res struct {
		Capabilities json.RawMessage `json:"capabilities"`
	}
	if err := json.Unmarshal(result, &res); err != nil {
		return serverFeatures{}
	}
	var caps struct {
		ExecuteCommand struct {
			Commands []string `json:"commands"`
		} `json:"executeCommandProvider"`
	}
	_ = json.Unmarshal(res.Capabilities, &caps)
	commands := make(map[string]bool, len(caps.ExecuteCommand.Commands))
	for _, c := range caps.ExecuteCommand.Commands {
		commands[c] = true
	}
	return serverFeatures{semTok: semtok.FromCapabilities(res.Capabilities), commands: commands}
}
