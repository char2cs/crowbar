package agents

import (
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

func (a *agent) PromptType() (PromptTyping, bool) {
	ps := a.spec.Presentation.PromptSubmit
	if ps == nil || ps.Type == nil {
		return PromptTyping{}, false
	}
	return PromptTyping{
		Paste:             ps.Type.Paste,
		Submit:            ps.Type.Submit,
		ComposerRowPrefix: ps.Type.Composer.RowPrefix,
		SkipPrefixes:      slices.Clone(ps.Type.SkipPrefixes),
	}, true
}

// CanType refuses a message holding the end-paste sequence rather than stripping
// it: a message must reach the CLI exactly as written or not at all.
func (a *agent) CanType(text string) bool {
	typing, ok := a.PromptType()
	if !ok || typing.Paste != spec.PasteBracketed || strings.Contains(text, pasteEnd) {
		return false
	}
	return !slices.ContainsFunc(typing.SkipPrefixes, func(prefix string) bool {
		return strings.HasPrefix(text, prefix)
	})
}

func (a *agent) PromptTypeBytes(text string) ([]byte, bool) {
	if !a.CanType(text) {
		return nil, false
	}
	typing, _ := a.PromptType()
	return []byte(pasteStart + text + pasteEnd + typing.Submit), true
}
