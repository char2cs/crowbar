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
		"hook wiring in config_injection": {
			"config_injection:\n  - pass_arg: { a: \"{hook_curl} {hook_url}/x\" }\n  - pass_arg: { a: b }\n",
			"config_injection:\n  - pass_arg: {a: b}\nhooks_injection:\n  - pass_arg: {a: \"{hook_curl} {hook_url}/x\"}\n",
			descriptorschema.Rewritten, "config_injection",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			up := upgrade(t, "id: x\n"+tc.in)
			assert.Equal(t, "id: x\nschema_version: 4\n"+tc.want, string(up.Raw))
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

// The relay posted for the hook itself; {hook_curl} reads its token and body
// from {tmp}/hook.curl, which the document must now write, once.
func TestUpgrade_TheRelayHookBecomesCurlWithItsConfig(t *testing.T) {
	relay := "{crowbar_hook} hook turn_stop --segment {segid} --provider {provider} {scope_flags}"
	up := upgrade(t, "id: x\nhooks_injection:\n  - pass_arg: { value: \"S="+relay+"\" }\n"+
		"  - pass_arg: { value: \"T={crowbar_hook} hook user_prompt --segment {segid}\" }\n")
	assert.Equal(t, "id: x\nschema_version: 4\nhooks_injection:\n"+
		"  - write_file:\n      path: \"{tmp}/hook.curl\"\n      content: |\n"+
		"        header = \"X-Crowbar-Runner-Token: {runner_token}\"\n"+
		"        header = \"Content-Type: application/json\"\n"+
		"        data-binary = \"@-\"\n        connect-timeout = 5\n"+
		"  - pass_arg: {value: \"S={hook_curl} {hook_url}/turn_stop\"}\n"+
		"  - pass_arg: {value: \"T={hook_curl} {hook_url}/user_prompt\"}\n", string(up.Raw))
	require.Len(t, up.Changes, 3)
	assert.Equal(t, "hooks_injection", up.Changes[2].Path)

	written := upgrade(t, "id: x\nhooks_injection:\n  - write_file: { path: \"{tmp}/hook.curl\", content: mine }\n"+
		"  - pass_arg: { value: \"S="+relay+"\" }\n")
	assert.Len(t, written.Changes, 1, "a config the document writes is its own")
}

// A key the upgrade removes takes no comment with it: a retired key's notes
// stay above what follows it, a moved key's notes travel to where it went.
func TestUpgrade_ARemovedKeyKeepsItsComments(t *testing.T) {
	up := upgrade(t, "id: x\nsession:\n  # why locate\n  locate: { root: \"~/x\" } # inline\n"+
		"  persists: turn_start\nruntime:\n  transport: api\n  api:\n    # why codes\n    session_lost_codes: [-32600]\n"+
		"    delivery: stdio\n")
	got := string(up.Raw)
	assert.Contains(t, got, "  # why locate\n  # inline\n  persists: turn_start\n")
	assert.Contains(t, got, "  refused:\n    # why codes\n    rpc_code: -32600\n")
}
