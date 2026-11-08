package descriptorschema

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// moveFirst carries the first item of key's list to destKey of the mapping at
// dest, which names one value where key named several.
func moveFirst(key, dest, destKey string) func(match) []Change {
	return func(at match) []Change {
		i := keyIndex(at.node, key)
		if i < 0 {
			return nil
		}
		line, list := at.node.Content[i].Line, at.node.Content[i+1]
		notes := remove(at.node, i)
		path := join(at.path, key)
		if list.Kind != yaml.SequenceNode || len(list.Content) == 0 {
			keepComments(at.node, i, notes)
			return []Change{{Kind: Retired, Path: path, Line: line, Note: key + " was empty and was removed"}}
		}
		target := ensure(at.root, dest)
		if j := keyIndex(target, destKey); j >= 0 {
			keepComments(target, j, notes)
			return []Change{{
				Kind: Retired, Path: path, Line: line,
				Note: fmt.Sprintf("%s was removed: %s.%s already says it", key, dest, destKey),
			}}
		}
		first := deepCopy(list.Content[0])
		first.HeadComment, first.LineComment, first.FootComment = "", "", ""
		insert(target, -1, destKey, first)
		keepComments(target, len(target.Content)-2, notes)
		note := fmt.Sprintf("%s moved to %s.%s: %s", key, dest, destKey, first.Value)
		if len(list.Content) > 1 {
			note += fmt.Sprintf("; %s names one code, so the other %d were dropped", dest, len(list.Content)-1)
		}
		return []Change{{Kind: Rewritten, Path: path, Line: line, Note: note}}
	}
}

// predatesStdio is an api block from before serve spoke over stdio: it has a
// serve argv and states no delivery.
func predatesStdio(api *yaml.Node) bool {
	return value(api, "serve") != nil && value(api, "delivery") == nil
}

// handshakeDefaults declares the params and notification Crowbar used to send
// on every handshake before a descriptor stated them.
func handshakeDefaults(at match) []Change {
	hs := value(at.node, "handshake")
	if !predatesStdio(at.node) || hs == nil || hs.Kind != yaml.MappingNode {
		return nil
	}
	var added []string
	if keyIndex(hs, "params") < 0 {
		info := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle}
		insert(info, -1, "name", scalar("crowbar"))
		insert(info, -1, "title", scalar("Crowbar"))
		insert(info, -1, "version", scalar("0.0.0"))
		params := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle}
		insert(params, -1, "clientInfo", info)
		insert(hs, -1, "params", params)
		added = append(added, "params")
	}
	if keyIndex(hs, "notify") < 0 {
		insert(hs, -1, "notify", scalar("initialized"))
		added = append(added, "notify")
	}
	if len(added) == 0 {
		return nil
	}
	return []Change{{
		Kind: Rewritten, Path: join(at.path, "handshake"), Line: hs.Line,
		Note: fmt.Sprintf("handshake %s added: what Crowbar sent on every handshake before they were declared",
			strings.Join(added, " and ")),
	}}
}

// stdioServe moves a serve that listened on {socket} to stdio, the only way
// Crowbar speaks to serve now: the argv naming the socket goes.
func stdioServe(at match) []Change {
	if !predatesStdio(at.node) {
		return nil
	}
	i := keyIndex(at.node, "serve")
	serve := at.node.Content[i+1]
	dropped := dropSocketArgs(serve)
	insert(at.node, i, "format", scalar("ndjson"))
	insert(at.node, i, "delivery", scalar("stdio"))
	note := "delivery: stdio and format: ndjson added: Crowbar speaks to serve over its stdin and stdout"
	if len(dropped) > 0 {
		note += fmt.Sprintf("; the retired {socket} argv %q was removed, so serve must answer on stdio with what is left",
			strings.Join(dropped, " "))
	}
	return []Change{{Kind: Rewritten, Path: join(at.path, "serve"), Line: serve.Line, Note: note}}
}

// dropSocketArgs removes each argv item naming {socket}, with the bare flag
// right before it that took it as its value.
func dropSocketArgs(serve *yaml.Node) []string {
	if serve.Kind != yaml.SequenceNode {
		return nil
	}
	var kept []*yaml.Node
	var dropped []string
	for _, item := range serve.Content {
		if !strings.Contains(item.Value, "{socket}") {
			kept = append(kept, item)
			continue
		}
		if n := len(kept); n > 0 && isBareFlag(kept[n-1].Value) && !strings.HasPrefix(item.Value, "-") {
			dropped = append(dropped, kept[n-1].Value)
			kept = kept[:n-1]
		}
		dropped = append(dropped, item.Value)
	}
	serve.Content = kept
	return dropped
}

func isBareFlag(arg string) bool {
	return strings.HasPrefix(arg, "-") && !strings.Contains(arg, "=") && !strings.Contains(arg, "{")
}
