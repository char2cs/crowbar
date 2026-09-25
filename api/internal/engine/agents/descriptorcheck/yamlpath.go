package descriptorcheck

import (
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// document answers "which line is this path on" over a parsed YAML tree.
type document struct {
	root *yaml.Node
}

var (
	lineRE    = regexp.MustCompile(`line (\d+)`)
	segmentRE = regexp.MustCompile(`^([^\[\]]*)((?:\[\d+\])*)$`)
	indexRE   = regexp.MustCompile(`\[(\d+)\]`)
)

// lookup returns the node at a path like "session.locate.glob[0]", or nil.
func (d document) lookup(path string) *yaml.Node {
	n := d.body()
	for _, seg := range strings.Split(path, ".") {
		if n == nil {
			return nil
		}
		m := segmentRE.FindStringSubmatch(seg)
		if m == nil {
			return nil
		}
		if m[1] != "" {
			n = mapValue(n, m[1])
		}
		for _, idx := range indexRE.FindAllStringSubmatch(m[2], -1) {
			i, _ := strconv.Atoi(idx[1])
			if n == nil || n.Kind != yaml.SequenceNode || i >= len(n.Content) {
				return nil
			}
			n = n.Content[i]
		}
	}
	return n
}

// line is the line of path, or of its nearest ancestor that exists.
func (d document) line(path string) int {
	for p := path; p != ""; p = parent(p) {
		if n := d.lookup(p); n != nil {
			return keyLine(d, p, n)
		}
	}
	return 0
}

// keyLine is the line of the key naming a mapping value, so a finding about a
// block points at its heading rather than its first child.
func keyLine(d document, path string, value *yaml.Node) int {
	up := d.body()
	if parent(path) != "" {
		up = d.lookup(parent(path))
	}
	if up == nil || up.Kind != yaml.MappingNode {
		return value.Line
	}
	for i := 0; i+1 < len(up.Content); i += 2 {
		if up.Content[i+1] == value {
			return up.Content[i].Line
		}
	}
	return value.Line
}

// pathAt names the mapping key on line, for a finding the decoder reported by
// line alone.
func (d document) pathAt(line int) string {
	return findLine(d.body(), "", line)
}

func (d document) body() *yaml.Node {
	if d.root == nil {
		return nil
	}
	if d.root.Kind == yaml.DocumentNode && len(d.root.Content) > 0 {
		return d.root.Content[0]
	}
	return d.root
}

func findLine(n *yaml.Node, prefix string, line int) string {
	if n == nil {
		return ""
	}
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := join(prefix, n.Content[i].Value)
			if n.Content[i].Line == line {
				return key
			}
			if p := findLine(n.Content[i+1], key, line); p != "" {
				return p
			}
		}
	case yaml.SequenceNode:
		for i, c := range n.Content {
			if p := findLine(c, prefix+"["+strconv.Itoa(i)+"]", line); p != "" {
				return p
			}
		}
	case yaml.DocumentNode, yaml.ScalarNode, yaml.AliasNode:
	}
	return ""
}

func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func join(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func parent(path string) string {
	if i := strings.LastIndexByte(path, '['); i > strings.LastIndexByte(path, '.') {
		return path[:i]
	}
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[:i]
	}
	return ""
}

func errorLine(msg string) int {
	m := lineRE.FindStringSubmatch(msg)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
