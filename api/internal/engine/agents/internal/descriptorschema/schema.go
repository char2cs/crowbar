// Package descriptorschema brings a descriptor document written for an older
// schema up to the current one. It is a table of transitions keyed by schema
// path and applied by structure alone, so any descriptor upgrades the same way.
package descriptorschema

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Current is the schema this build reads. A schema change bumps it and adds
// the transitions that carry an older document over.
const Current = 3

// oldest is what an unstamped document is taken to be: every transition is
// considered, and each one acts only on the shape it retires.
const oldest = 1

// VersionKey is the top-level key a document states its schema in.
const VersionKey = "schema_version"

// ErrNewer is a document stamped with a schema this build does not know.
var ErrNewer = errors.New("descriptor schema is newer than this build")

// Kind says what a change did to the document.
type Kind string

const (
	// Rewritten carries a key or value over to the shape that replaced it.
	Rewritten Kind = "rewritten"
	// Retired drops a key that means nothing any more.
	Retired Kind = "retired"
)

// Change is one thing an upgrade did, at the path and line of the original.
type Change struct {
	Kind  Kind
	Path  string
	Line  int
	Since int
	Note  string
}

// Result is a document brought to Current: Raw is the input when nothing
// changed, From the schema the input was read as.
type Result struct {
	Raw     []byte
	From    int
	Changes []Change
}

// transition is one schema change: at every node paths name, apply rewrites
// the old shape it finds there and says what it did.
type transition struct {
	since int
	paths []string
	apply func(at match) []Change
}

// The places a canonical field map, an event discriminator and a wire name sit.
var (
	fieldMaps   = []string{"events.*.map", "events.*.api.map", "events.*.hooks.map"}
	whenMaps    = []string{"events.*.when", "events.*.api.when", "events.*.hooks.when"}
	eventBlocks = []string{"events.*", "events.*.api", "events.*.hooks"}
)

var transitions = []transition{
	// 3: the `crowbar hook` relay is gone; hooks post to the daemon. First, so
	// the step below sees the hook commands it wrote.
	{3, []string{""}, everyScalar(relayHook)},
	// 2: hook wiring moved out of config_injection, which also reaches serve.
	{2, []string{""}, moveSteps("config_injection", "hooks_injection", "{hook_curl} ")},
	// 2: the one `||` glyph became one spelling per job, in the same order.
	{2, fieldMaps, eachValue(alternation("first_present"))},
	{2, whenMaps, eachValue(alternation("any_of"))},
	{2, eventBlocks, wireNames("in", "out", "ask")},

	// 3: subagents became ledger scopes.
	{3, fieldMaps, renameKey("subagent_id", "scope_id")},
	{3, fieldMaps, renameKey("nested_session_id", "spawns_scope", "addresses_scope")},
	{3, fieldMaps, retireKey("async_work", "a turn's background work is read from its live subagents")},
	// 3: the chat owns its session id; a lost session is the CLI's refusal.
	{3, []string{"session"}, retireKey("locate",
		"Crowbar keeps the chat's session id itself and learns a lost session from session.refused")},
	{3, []string{"session"}, defaultKey("resume", "persists", "turn_start")},
	{3, []string{"runtime.api"}, moveFirst("session_lost_codes", "session.refused", "rpc_code")},
	// 3: serve speaks over stdio and the handshake is declared, not built in.
	{3, []string{"runtime.api"}, handshakeDefaults},
	{3, []string{"runtime.api"}, stdioServe},
}

// Upgrade brings raw to Current. A document that does not parse, or is not a
// mapping, is returned as is for the parser to report.
func Upgrade(raw []byte) (Result, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil || len(root.Content) == 0 {
		return Result{Raw: raw, From: Current}, nil //nolint:nilerr // a YAML error is the parser's to report
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return Result{Raw: raw, From: Current}, nil
	}
	from, err := stamp(doc)
	if err != nil {
		return Result{Raw: raw, From: from}, err
	}
	var changes []Change
	for _, t := range transitions {
		if t.since <= from {
			continue
		}
		for _, path := range t.paths {
			for _, at := range find(doc, path) {
				for _, c := range t.apply(at) {
					c.Since = t.since
					changes = append(changes, c)
				}
			}
		}
	}
	if len(changes) == 0 {
		return Result{Raw: raw, From: from}, nil
	}
	setStamp(doc)
	out, err := encode(&root)
	if err != nil {
		return Result{Raw: raw, From: from}, err
	}
	return Result{Raw: out, From: from, Changes: changes}, nil
}

// stamp is the document's schema_version, oldest when it states none.
func stamp(doc *yaml.Node) (int, error) {
	n := value(doc, VersionKey)
	if n == nil {
		return oldest, nil
	}
	v, err := strconv.Atoi(n.Value)
	if err != nil || v < oldest {
		return oldest, fmt.Errorf("descriptor: %s %q (line %d) is not a schema number", VersionKey, n.Value, n.Line)
	}
	if v > Current {
		return v, fmt.Errorf("%w: %s %d (line %d), this build reads up to %d", ErrNewer, VersionKey, v, n.Line, Current)
	}
	return v, nil
}

// setStamp states Current, right after id: when the document names one.
func setStamp(doc *yaml.Node) {
	if n := value(doc, VersionKey); n != nil {
		n.Value, n.Tag, n.Style = strconv.Itoa(Current), "!!int", 0
		return
	}
	at := 0
	if i := keyIndex(doc, "id"); i >= 0 {
		at = i + 2
	}
	key, val := scalar(VersionKey), scalar(strconv.Itoa(Current))
	val.Tag = "!!int"
	doc.Content = append(doc.Content[:at], append([]*yaml.Node{key, val}, doc.Content[at:]...)...)
}

func encode(root *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("descriptor: encode upgraded document: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("descriptor: encode upgraded document: %w", err)
	}
	return buf.Bytes(), nil
}
