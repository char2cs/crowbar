package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/pathselect"
)

type Exec func(ctx context.Context, argv, env []string) ([]byte, error)

type Fields struct {
	URL        string
	Protocols  string
	Headers    map[string]string
	Credential string
	Schemes    []string
}

type Endpoint struct {
	URL       string
	Protocols []string
	Header    http.Header
}

func Resolve(ctx context.Context, run Exec, argv, env []string, f Fields, allowed []string) (Endpoint, error) {
	name := "connect"
	if len(argv) > 0 {
		name = argv[0]
	}
	for header := range f.Headers {
		if !slices.ContainsFunc(allowed, func(a string) bool { return http.CanonicalHeaderKey(a) == http.CanonicalHeaderKey(header) }) {
			return Endpoint{}, fmt.Errorf("connect: %s: header %q is not allowed", name, header)
		}
	}
	out, err := run(ctx, argv, env)
	if err != nil {
		return Endpoint{}, fmt.Errorf("connect: %s: run: %w", name, err)
	}
	doc, err := decodeOne(out)
	if err != nil {
		return Endpoint{}, fmt.Errorf("connect: %s: %w", name, err)
	}
	ep, err := mapFields(doc, f)
	if err != nil {
		return Endpoint{}, fmt.Errorf("connect: %s: %w", name, err)
	}
	return ep, nil
}

func decodeOne(out []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(out))
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil || doc == nil {
		return nil, errors.New("decode: stdout is not a JSON object")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("stdout must be exactly one JSON object")
	}
	return doc, nil
}

func mapFields(doc map[string]any, f Fields) (Endpoint, error) {
	raw, ok := pathselect.Field(doc, f.URL).(string)
	schemes := f.Schemes
	if len(schemes) == 0 {
		schemes = []string{"ws", "wss"}
	}
	if u, perr := url.Parse(raw); !ok || perr != nil || !slices.Contains(schemes, u.Scheme) {
		return Endpoint{}, fmt.Errorf("field %q: want a %s URL string", f.URL, strings.Join(schemes, " or "))
	}
	ep := Endpoint{URL: raw, Header: http.Header{}}
	if f.Protocols != "" {
		list, ok := stringList(pathselect.Field(doc, f.Protocols))
		if !ok {
			return Endpoint{}, fmt.Errorf("field %q: want an array of strings", f.Protocols)
		}
		ep.Protocols = list
	}
	for header, path := range f.Headers {
		if v, ok := pathselect.Field(doc, path).(string); ok {
			ep.Header.Set(header, v)
		}
	}
	if f.Credential != "" {
		v, ok := pathselect.Field(doc, f.Credential).(string)
		if !ok || v == "" {
			return Endpoint{}, fmt.Errorf("field %q: want a non-empty string", f.Credential)
		}
		ep.Header.Set("Authorization", "Bearer "+v)
	}
	return ep, nil
}

func stringList(v any) ([]string, bool) {
	if v == nil {
		return nil, true
	}
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}
