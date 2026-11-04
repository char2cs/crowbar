package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

func (s *server) Replay(
	ctx context.Context,
) error {
	if s.spawn == nil {
		return ErrNoSpawn
	}
	transport, err := s.spawn(ctx)
	if err != nil {
		return fmt.Errorf("replay: spawn: %w", err)
	}
	s.swapTransport(transport)

	s.mu.Lock()
	rootDir := s.rootDir
	s.mu.Unlock()
	if rootDir != "" {
		if err := s.handshake(ctx, rootDir); err != nil {
			return fmt.Errorf("replay: %w", err)
		}
	}

	for _, uri := range s.openURIs() {
		params := s.didOpenParams(uri)
		if err := s.Notify(ctx, "textDocument/didOpen", params); err != nil {
			return fmt.Errorf("replay: didOpen %s: %w", uri, err)
		}
	}
	return nil
}

func (s *server) openURIs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	uris := make([]string, 0, len(s.openParams))
	for uri := range s.openParams {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	return uris
}

func (s *server) didOpenParams(
	uri string,
) json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openParams[uri]
}

// trackDoc keeps the replay state for open documents: didOpen records the
// params, didChange folds the new full text into them (so a restart reopens
// what the editor holds now, not what it held at open), didClose drops them.
func (s *server) trackDoc(
	method string,
	params json.RawMessage,
) {
	switch method {
	case "textDocument/didOpen", "textDocument/didClose", "textDocument/didChange":
	default:
		return
	}
	var p struct {
		TextDocument struct {
			URI     string `json:"uri"`
			Version int    `json:"version"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Range *json.RawMessage `json:"range"`
			Text  string           `json:"text"`
		} `json:"contentChanges"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	uri := p.TextDocument.URI
	if uri == "" {
		return
	}
	switch method {
	case "textDocument/didOpen":
		s.docs.Add(uri)
		s.mu.Lock()
		s.openParams[uri] = params
		s.mu.Unlock()
	case "textDocument/didClose":
		s.docs.Remove(uri)
		s.mu.Lock()
		delete(s.openParams, uri)
		s.mu.Unlock()
	case "textDocument/didChange":
		// Only full-document syncs are sent; an incremental change (with a
		// range) cannot be folded without the base text, so it is ignored.
		if len(p.ContentChanges) != 1 || p.ContentChanges[0].Range != nil {
			return
		}
		s.foldChange(uri, p.TextDocument.Version, p.ContentChanges[0].Text)
	}
}

func (s *server) foldChange(
	uri string,
	version int,
	text string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	open, ok := s.openParams[uri]
	if !ok {
		return
	}
	var doc struct {
		TextDocument map[string]any `json:"textDocument"`
	}
	if err := json.Unmarshal(open, &doc); err != nil || doc.TextDocument == nil {
		return
	}
	doc.TextDocument["text"] = text
	if version > 0 {
		doc.TextDocument["version"] = version
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return
	}
	s.openParams[uri] = raw
}
