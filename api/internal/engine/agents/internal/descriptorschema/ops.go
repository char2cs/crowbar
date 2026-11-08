package descriptorschema

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// match is a node a transition's path named, with that path and the root a
// transition moving a key elsewhere writes into.
type match struct {
	node *yaml.Node
	path string
	root *yaml.Node
}

// find resolves a dotted path, "*" standing for every key of a mapping.
func find(doc *yaml.Node, path string) []match {
	out := []match{{node: doc, root: doc}}
	if path == "" {
		return out
	}
	for _, seg := range strings.Split(path, ".") {
		var next []match
		for _, m := range out {
			if m.node.Kind != yaml.MappingNode {
				continue
			}
			for i := 0; i+1 < len(m.node.Content); i += 2 {
				if key := m.node.Content[i].Value; seg == "*" || key == seg {
					next = append(next, match{node: m.node.Content[i+1], path: join(m.path, key), root: doc})
				}
			}
		}
		out = next
	}
	return out
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func keyIndex(m *yaml.Node, key string) int {
	if m == nil || m.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func value(m *yaml.Node, key string) *yaml.Node {
	if i := keyIndex(m, key); i >= 0 {
		return m.Content[i+1]
	}
	return nil
}

func scalar(v string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
	if strings.ContainsAny(v, "[]{},:#&*!|>'\"%@`") || v != strings.TrimSpace(v) {
		n.Style = yaml.DoubleQuotedStyle
	}
	return n
}

// insert puts key: val into m at pair index at (a Content index), or at the end.
func insert(m *yaml.Node, at int, key string, val *yaml.Node) {
	if at < 0 || at > len(m.Content) {
		at = len(m.Content)
	}
	m.Content = append(m.Content[:at], append([]*yaml.Node{scalar(key), val}, m.Content[at:]...)...)
}

// remove deletes m's i-th key and returns every comment it carried, so the
// caller can keep them where the key's meaning went.
func remove(m *yaml.Node, i int) string {
	notes := comments(m.Content[i], m.Content[i+1])
	m.Content = append(m.Content[:i], m.Content[i+2:]...)
	return notes
}

func comments(nodes ...*yaml.Node) string {
	var out []string
	for _, n := range nodes {
		for _, c := range []string{n.HeadComment, n.LineComment} {
			if c != "" {
				out = append(out, c)
			}
		}
		if inner := comments(n.Content...); inner != "" {
			out = append(out, inner)
		}
		if n.FootComment != "" {
			out = append(out, n.FootComment)
		}
	}
	return strings.Join(out, "\n")
}

// keepComments sets notes above the key now at i of m, or under m when no key
// follows.
func keepComments(m *yaml.Node, i int, notes string) {
	if notes == "" {
		return
	}
	if i < len(m.Content) {
		m.Content[i].HeadComment = strings.TrimSuffix(notes+"\n"+m.Content[i].HeadComment, "\n")
		return
	}
	m.FootComment = strings.TrimPrefix(m.FootComment+"\n"+notes, "\n")
}

// eachValue applies f to every value of the matched mapping.
func eachValue(f func(path string, v *yaml.Node) []Change) func(match) []Change {
	return func(at match) []Change {
		if at.node.Kind != yaml.MappingNode {
			return nil
		}
		var out []Change
		for i := 0; i+1 < len(at.node.Content); i += 2 {
			out = append(out, f(join(at.path, at.node.Content[i].Value), at.node.Content[i+1])...)
		}
		return out
	}
}

// glyphParts splits a `||` alternation, nil when v is not one.
func glyphParts(v *yaml.Node) []string {
	if v.Kind != yaml.ScalarNode || !strings.Contains(v.Value, "||") {
		return nil
	}
	var parts []string
	for _, p := range strings.Split(v.Value, "||") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func flowSeq(items []string) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, it := range items {
		seq.Content = append(seq.Content, scalar(it))
	}
	return seq
}

// alternation rewrites `a || b` as {operator: [a, b]}: the same branches, tried
// in the same order.
func alternation(operator string) func(path string, v *yaml.Node) []Change {
	return func(path string, v *yaml.Node) []Change {
		parts := glyphParts(v)
		if parts == nil {
			return nil
		}
		was, line, comment := v.Value, v.Line, v.LineComment
		*v = yaml.Node{
			Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle, LineComment: comment,
			Content: []*yaml.Node{scalar(operator), flowSeq(parts)},
		}
		return []Change{{
			Kind: Rewritten, Path: path, Line: line,
			Note: fmt.Sprintf("%q became { %s: [%s] }: the || glyph is retired", was, operator, strings.Join(parts, ", ")),
		}}
	}
}

// wireNames rewrites a `a || b` wire name as the list [a, b].
func wireNames(keys ...string) func(match) []Change {
	return func(at match) []Change {
		var out []Change
		for _, key := range keys {
			v := value(at.node, key)
			parts := []string(nil)
			if v != nil {
				parts = glyphParts(v)
			}
			if parts == nil {
				continue
			}
			was, line := v.Value, v.Line
			*v = *flowSeq(parts)
			out = append(out, Change{
				Kind: Rewritten, Path: join(at.path, key), Line: line,
				Note: fmt.Sprintf("%q became the list [%s]: the || glyph is retired", was, strings.Join(parts, ", ")),
			})
		}
		return out
	}
}

// renameKey carries from's value to every name in to; a name already present
// keeps its own value.
func renameKey(from string, to ...string) func(match) []Change {
	return func(at match) []Change {
		i := keyIndex(at.node, from)
		if i < 0 {
			return nil
		}
		key, val := at.node.Content[i], at.node.Content[i+1]
		var added []string
		for n := len(to) - 1; n >= 0; n-- {
			if keyIndex(at.node, to[n]) >= 0 {
				continue
			}
			added = append([]string{to[n]}, added...)
			insert(at.node, i+2, to[n], deepCopy(val))
		}
		notes := remove(at.node, i)
		if len(added) == 0 {
			keepComments(at.node, i, notes)
			return []Change{{
				Kind: Retired, Path: join(at.path, from), Line: key.Line,
				Note: fmt.Sprintf("%s was removed: %s already says it", from, strings.Join(to, " and ")),
			}}
		}
		at.node.Content[i].HeadComment, at.node.Content[i].LineComment = key.HeadComment, key.LineComment
		return []Change{{
			Kind: Rewritten, Path: join(at.path, from), Line: key.Line,
			Note: fmt.Sprintf("%s is now %s, reading the same field", from, strings.Join(added, " and ")),
		}}
	}
}

// retireKey drops a key that has no meaning any more.
func retireKey(key, why string) func(match) []Change {
	return func(at match) []Change {
		i := keyIndex(at.node, key)
		if i < 0 {
			return nil
		}
		line := at.node.Content[i].Line
		keepComments(at.node, i, remove(at.node, i))
		return []Change{{
			Kind: Retired, Path: join(at.path, key), Line: line,
			Note: fmt.Sprintf("%s is ignored and was removed: %s", key, why),
		}}
	}
}

// defaultKey states key: val, the meaning its absence had, wherever when is
// declared and key is not.
func defaultKey(when, key, val string) func(match) []Change {
	return func(at match) []Change {
		i := keyIndex(at.node, when)
		if i < 0 || keyIndex(at.node, key) >= 0 {
			return nil
		}
		line := at.node.Content[i].Line
		insert(at.node, i+2, key, scalar(val))
		return []Change{{
			Kind: Rewritten, Path: join(at.path, key), Line: line,
			Note: fmt.Sprintf("%s: %s added, what leaving it out meant", key, val),
		}}
	}
}

func deepCopy(n *yaml.Node) *yaml.Node {
	c := *n
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, child := range n.Content {
		c.Content[i] = deepCopy(child)
	}
	return &c
}

// ensure is the mapping at a dotted path from root, created as needed.
func ensure(root *yaml.Node, path string) *yaml.Node {
	m := root
	for _, seg := range strings.Split(path, ".") {
		next := value(m, seg)
		if next == nil || next.Kind != yaml.MappingNode {
			next = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			setValue(m, seg, next)
		}
		m = next
	}
	return m
}

// setValue replaces key's value in m, or adds key: v at the end.
func setValue(m *yaml.Node, key string, v *yaml.Node) {
	if i := keyIndex(m, key); i >= 0 {
		m.Content[i+1] = v
		return
	}
	insert(m, -1, key, v)
}

// relayRE is the retired relay command and the scope flags it carried, all of
// which {hook_url} names now, up to the end of that command.
var relayRE = regexp.MustCompile(`\{crowbar_hook\} hook ([a-z_]+)` +
	`((?: --(?:segment|provider|project|repo|workspace|home)[ =](?:\{[a-z_]+\}|[A-Za-z0-9_./-]+)| \{scope_flags\})*)` +
	`($|["'\\;&|])`)

// hookCurlConfig is what {hook_curl} reads: the runner's token and the hook
// payload on stdin, which the relay used to carry itself.
const hookCurlConfig = `header = "X-Crowbar-Runner-Token: {runner_token}"
header = "Content-Type: application/json"
data-binary = "@-"
connect-timeout = 5
`

// relayHooks rewrites every relay command and, unless the document writes
// {tmp}/hook.curl itself, writes it first in hooks_injection.
func relayHooks(at match) []Change {
	changes := everyScalar(relayHook)(at)
	if len(changes) == 0 || mentions(at.node, "{tmp}/hook.curl") {
		return changes
	}
	step := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	file := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	insert(file, -1, "path", scalar("{tmp}/hook.curl"))
	insert(file, -1, "content", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Style: yaml.LiteralStyle, Value: hookCurlConfig})
	insert(step, -1, "write_file", file)
	list := value(at.node, "hooks_injection")
	if list == nil || list.Kind != yaml.SequenceNode {
		list = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		setValue(at.node, "hooks_injection", list)
	}
	list.Content = append([]*yaml.Node{step}, list.Content...)
	return append(changes, Change{
		Kind: Rewritten, Path: "hooks_injection", Line: list.Line,
		Note: "a write_file of {tmp}/hook.curl added: {hook_curl} reads the runner token and the payload from it",
	})
}

func relayHook(path string, v *yaml.Node) []Change {
	if !relayRE.MatchString(v.Value) {
		return nil
	}
	v.Value = relayRE.ReplaceAllString(v.Value, "{hook_curl} {hook_url}/${1}${3}")
	return []Change{{
		Kind: Rewritten, Path: path, Line: v.Line,
		Note: "the `{crowbar_hook} hook <event>` relay became `{hook_curl} {hook_url}/<event>`: hooks post to the daemon",
	}}
}

// everyScalar applies f to every scalar value under the match, keys aside.
func everyScalar(f func(path string, v *yaml.Node) []Change) func(match) []Change {
	var walk func(path string, n *yaml.Node) []Change
	walk = func(path string, n *yaml.Node) []Change {
		if n.Kind == yaml.ScalarNode {
			return f(path, n)
		}
		var out []Change
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				out = append(out, walk(join(path, n.Content[i].Value), n.Content[i+1])...)
			}
		}
		if n.Kind == yaml.SequenceNode {
			for i, item := range n.Content {
				out = append(out, walk(fmt.Sprintf("%s[%d]", path, i), item)...)
			}
		}
		return out
	}
	return func(at match) []Change { return walk(at.path, at.node) }
}

// moveSteps moves each step of the from list whose text contains marker to the
// end of the to list, creating it when absent.
func moveSteps(from, to, marker string) func(match) []Change {
	return func(at match) []Change {
		list := value(at.node, from)
		if list == nil || list.Kind != yaml.SequenceNode {
			return nil
		}
		var kept, moved []*yaml.Node
		for _, step := range list.Content {
			if mentions(step, marker) {
				moved = append(moved, step)
				continue
			}
			kept = append(kept, step)
		}
		if len(moved) == 0 {
			return nil
		}
		list.Content = kept
		dest := value(at.node, to)
		if dest == nil || dest.Kind != yaml.SequenceNode {
			dest = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			insert(at.node, keyIndex(at.node, from)+2, to, dest)
		}
		dest.Content = append(dest.Content, moved...)
		return []Change{{
			Kind: Rewritten, Path: join(at.path, from), Line: moved[0].Line,
			Note: fmt.Sprintf("%d step(s) wiring hooks moved to %s, which reaches the terminal process alone", len(moved), to),
		}}
	}
}

func mentions(n *yaml.Node, marker string) bool {
	if n.Kind == yaml.ScalarNode {
		return strings.Contains(n.Value, marker)
	}
	for _, c := range n.Content {
		if mentions(c, marker) {
			return true
		}
	}
	return false
}
