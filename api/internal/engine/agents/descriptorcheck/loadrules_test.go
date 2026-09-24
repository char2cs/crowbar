package descriptorcheck_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

func TestValidate_ALoadRuleIsReportedWithItsRuleAndPath(t *testing.T) {
	rep := descriptorcheck.Validate([]byte("id: bare\nspawn:\n  cmd: bare\n"))
	f := findingFor(t, rep, "load.spawn_command")
	assert.Equal(t, "spawn", f.Path)
	assert.Equal(t, 2, f.Line)
	assert.NotEmpty(t, f.Hint)
}

// A vocabulary failure names its event, so the finding points at it.
func TestValidate_AnEventOutsideTheVocabularyPointsAtTheEvent(t *testing.T) {
	doc := minimal("events:\n  turn_stopped:\n    in: Stop\n    map: { session_id: session_id }\n")
	rep := descriptorcheck.Validate(doc)
	f := findingFor(t, rep, "load.parse")
	assert.Equal(t, "events.turn_stopped", f.Path)
	assert.Equal(t, lineOf(t, doc, "  turn_stopped:"), f.Line)
	assert.NotEmpty(t, f.Hint)
}
