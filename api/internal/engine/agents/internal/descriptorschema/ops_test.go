package descriptorschema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptorschema"
)

func TestUpgrade_EachRetiredShapeBecomesItsReplacement(t *testing.T) {
	cases := map[string]struct {
		in, want string
		kind     descriptorschema.Kind
		path     string
	}{
		"alternation in a map": {
			"events:\n  e:\n    map: { f: \"a.b || c\" }\n",
			"events:\n  e:\n    map: {f: {first_present: [a.b, c]}}\n",
			descriptorschema.Rewritten, "events.e.map.f",
		},
		"a path with a selector stays quoted": {
			"events:\n  e:\n    map: { f: \"x[0] || y\" }\n",
			"events:\n  e:\n    map: {f: {first_present: [\"x[0]\", y]}}\n",
			descriptorschema.Rewritten, "events.e.map.f",
		},
		"alternation in a channel block's when": {
			"events:\n  e:\n    api:\n      when: { t: a || b }\n",
			"events:\n  e:\n    api:\n      when: {t: {any_of: [a, b]}}\n",
			descriptorschema.Rewritten, "events.e.api.when.t",
		},
		"alternation in a wire name": {
			"events:\n  e:\n    ask: a/b || c/d\n",
			"events:\n  e:\n    ask: [a/b, c/d]\n",
			descriptorschema.Rewritten, "events.e.ask",
		},
		"a renamed field": {
			"events:\n  e:\n    hooks:\n      map: { subagent_id: agent_id, x: y }\n",
			"events:\n  e:\n    hooks:\n      map: {scope_id: agent_id, x: y}\n",
			descriptorschema.Rewritten, "events.e.hooks.map.subagent_id",
		},
		"a field split in two": {
			"events:\n  e:\n    map: { nested_session_id: p }\n",
			"events:\n  e:\n    map: {spawns_scope: p, addresses_scope: p}\n",
			descriptorschema.Rewritten, "events.e.map.nested_session_id",
		},
		"a field that means nothing now": {
			"events:\n  e:\n    map: { async_work: b, m: n }\n",
			"events:\n  e:\n    map: {m: n}\n",
			descriptorschema.Retired, "events.e.map.async_work",
		},
		"the session locator": {
			"session:\n  locate: { glob: [x] }\n  persists: turn_end\n",
			"session:\n  persists: turn_end\n",
			descriptorschema.Retired, "session.locate",
		},
		"persists states its default": {
			"session:\n  resume: { arg: \"-r {id}\" }\n",
			"session:\n  resume: {arg: \"-r {id}\"}\n  persists: turn_start\n",
			descriptorschema.Rewritten, "session.persists",
		},
		"the relay hook": {
			"hooks_injection:\n  - pass_arg: { value: \"S={crowbar_hook} hook turn_stop --segment {segid} --provider {provider} {scope_flags}\" }\n",
			"hooks_injection:\n  - pass_arg: {value: \"S={hook_curl} {hook_url}/turn_stop\"}\n",
			descriptorschema.Rewritten, "hooks_injection[0].pass_arg.value",
		},
		"hook wiring in config_injection": {
			"config_injection:\n  - pass_arg: { a: \"{hook_curl} {hook_url}/x\" }\n  - pass_arg: { a: b }\n",
			"config_injection:\n  - pass_arg: {a: b}\nhooks_injection:\n  - pass_arg: {a: \"{hook_curl} {hook_url}/x\"}\n",
			descriptorschema.Rewritten, "config_injection",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			up := upgrade(t, "id: x\n"+tc.in)
			assert.Equal(t, "id: x\nschema_version: 3\n"+tc.want, string(up.Raw))
			require.Len(t, up.Changes, 1)
			assert.Equal(t, tc.kind, up.Changes[0].Kind)
			assert.Equal(t, tc.path, up.Changes[0].Path)
			assert.Positive(t, up.Changes[0].Line)
			assert.NotEmpty(t, up.Changes[0].Note)
		})
	}
}

// A relay line carrying a flag the hook URL does not stand for is left for
// the validator to name, never half rewritten.
func TestUpgrade_ARelayLineWithAnUnknownFlagIsLeftAlone(t *testing.T) {
	in := "id: x\nhooks_injection:\n  - pass_arg: { value: \"{crowbar_hook} hook turn_stop --payload-file /p\" }\n"
	up := upgrade(t, in)
	assert.Contains(t, string(up.Raw), "{crowbar_hook} hook turn_stop --payload-file /p")
}
