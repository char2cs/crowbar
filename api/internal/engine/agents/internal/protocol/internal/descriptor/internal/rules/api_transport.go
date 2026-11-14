package rules

import (
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// apiTransport checks how each delivery kind reaches its server: stdio opens
// serve's pipes before the fork, websocket and http dial what connect prints.
type apiTransport struct{}

func (apiTransport) Name() string { return "api_transport" }

var (
	envNameRE    = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	nameRE       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	pathRE       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\[\]]*$`)
	headerNameRE = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
	placeholder  = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_.]*)\}`)
	// Values Crowbar itself supplies to a request body or path; text and ask_id are
	// the prompt's and the asked event's own; call_id is a fresh id minted per call.
	requestInputs = []string{"prompt", "message", "text", "session_id", "cwd", "model", "effort", "ask_id", "call_id"}
)

func (apiTransport) Check(d *spec.Descriptor) error {
	api := d.Runtime.API
	if err := checkRemember(d); err != nil {
		return err
	}
	if len(api.Serve) == 0 && api.Delivery == "" && api.Format == "" && len(api.Connect) == 0 && api.StaticURL == "" {
		return nil
	}
	switch api.Delivery {
	case spec.APIDeliveryWebsocket:
		return checkWebsocket(d)
	case spec.APIDeliveryHTTP:
		return checkHTTP(d)
	case spec.APIDeliveryStdio:
		return checkStdio(d)
	}
	if len(api.Serve) == 0 {
		return nil
	}
	return invalid(d.ID, "runtime.api.delivery must be stdio, websocket or http, got %q", api.Delivery)
}

func checkStdio(d *spec.Descriptor) error {
	api := d.Runtime.API
	if len(api.Serve) == 0 {
		return invalid(d.ID, "runtime.api.delivery stdio requires runtime.api.serve")
	}
	if api.Format != spec.APIFormatNDJSON {
		return invalid(d.ID, "runtime.api.format must be %q, got %q", spec.APIFormatNDJSON, api.Format)
	}
	return nil
}

func checkEndpoint(d *spec.Descriptor, format string) error {
	api := d.Runtime.API
	if len(api.Serve) > 0 {
		return invalid(d.ID, "runtime.api.serve is only for delivery stdio, not %q", api.Delivery)
	}
	if api.Format != format {
		return invalid(d.ID, "runtime.api.format must be %q, got %q", format, api.Format)
	}
	return checkConnect(d)
}

// checkConnect validates how the endpoint is named: connect argv or a static URL.
func checkConnect(d *spec.Descriptor) error {
	api := d.Runtime.API
	if len(api.Connect) == 0 && api.StaticURL == "" {
		return invalid(d.ID, "runtime.api needs connect or static_url to name an endpoint")
	}
	if len(api.Connect) > 0 && api.StaticURL != "" {
		return invalid(d.ID, "runtime.api.connect and static_url are exclusive")
	}
	if len(api.Connect) > 0 && (hasEmptyArg(api.Connect) || api.ConnectMap.URL == "") {
		return invalid(d.ID, "runtime.api.connect needs non-empty argv and connect_map.url")
	}
	if len(api.Connect) == 0 && (api.ConnectMap.URL != "" || api.ConnectMap.Protocols != "" || api.ConnectMap.Credential != "" || len(api.ConnectMap.Headers) > 0) {
		return invalid(d.ID, "runtime.api.connect_map has no connect output to read")
	}
	return checkConnectHeaders(d)
}

func checkConnectHeaders(d *spec.Descriptor) error {
	api := d.Runtime.API
	for name, path := range api.ConnectMap.Headers {
		if !headerNameRE.MatchString(name) || !pathRE.MatchString(path) {
			return invalid(d.ID, "runtime.api.connect_map.headers[%q]: invalid header name or path %q", name, path)
		}
	}
	return nil
}

func checkWebsocket(d *spec.Descriptor) error {
	if err := checkEndpoint(d, spec.APIFormatJSONRPCText); err != nil {
		return err
	}
	if d.Runtime.API.Auth != (spec.AuthSpec{}) || len(d.Runtime.API.Requests) > 0 || d.Runtime.API.Stream.Path != "" {
		return invalid(d.ID, "runtime.api auth, requests and stream are only for delivery http")
	}
	return nil
}

func checkHTTP(d *spec.Descriptor) error {
	api := d.Runtime.API
	if err := checkEndpoint(d, spec.APIFormatSSE); err != nil {
		return err
	}
	if len(api.Requests) == 0 || api.Stream.Path == "" {
		return invalid(d.ID, "runtime.api delivery http requires requests and stream.path")
	}
	if err := checkAuth(d); err != nil {
		return err
	}
	captured := map[string]bool{}
	for _, r := range api.Requests {
		for name := range r.Capture {
			captured[name] = true
		}
	}
	for _, e := range d.Events {
		for _, steps := range [][]spec.CallStep{e.Fresh, e.Resume, e.Action} {
			for _, s := range steps {
				for name := range s.Send {
					captured[name] = true
				}
			}
		}
	}
	replies := map[string]bool{}
	for _, e := range d.Events {
		replies[e.ReplyRequest] = true
	}
	for name, r := range api.Requests {
		// A reply request is filled from the rendered decision's own members.
		if err := checkRequest(d, name, r, captured, replies[name]); err != nil {
			return err
		}
	}
	if err := checkPlaceholders(d, "stream.path", api.Stream.Path, captured); err != nil {
		return err
	}
	return checkRequestRefs(d)
}

func checkAuth(d *spec.Descriptor) error {
	a := d.Runtime.API.Auth
	if a.BearerEnv != "" && a.BearerPath != "" {
		return invalid(d.ID, "runtime.api.auth sets bearer_env and bearer_path; pick one")
	}
	if a.BearerEnv != "" && !envNameRE.MatchString(a.BearerEnv) {
		return invalid(d.ID, "runtime.api.auth.bearer_env must name an environment variable, not hold a secret")
	}
	if a.BearerPath != "" && (!pathRE.MatchString(a.BearerPath) || len(d.Runtime.API.Connect) == 0) {
		return invalid(d.ID, "runtime.api.auth.bearer_path must be a path into connect output, not hold a secret")
	}
	return nil
}

func checkRequest(d *spec.Descriptor, name string, r spec.RequestSpec, captured map[string]bool, free bool) error {
	if !slices.Contains([]string{"GET", "POST", "PUT", "PATCH", "DELETE"}, r.Method) {
		return invalid(d.ID, "runtime.api.requests[%s].method %q is not an http method", name, r.Method)
	}
	if !strings.HasPrefix(r.Path, "/") {
		return invalid(d.ID, "runtime.api.requests[%s].path must start with /", name)
	}
	for _, code := range r.OKStatus {
		if code < http.StatusOK || code > 599 {
			return invalid(d.ID, "runtime.api.requests[%s].ok_status %d is not an http status", name, code)
		}
	}
	if free {
		return nil
	}
	if err := checkPlaceholders(d, "requests["+name+"].path", r.Path, captured); err != nil {
		return err
	}
	return checkTree(d, "requests["+name+"].body", r.Body, captured)
}

func checkTree(d *spec.Descriptor, where string, v any, captured map[string]bool) error {
	switch t := v.(type) {
	case string:
		return checkPlaceholders(d, where, t, captured)
	case map[string]any:
		for k, e := range t {
			if err := checkTree(d, where+"."+k, e, captured); err != nil {
				return err
			}
		}
	case []any:
		for _, e := range t {
			if err := checkTree(d, where, e, captured); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkPlaceholders(d *spec.Descriptor, where, s string, captured map[string]bool) error {
	for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
		name := m[1]
		if captured[name] || slices.Contains(requestInputs, name) {
			continue
		}
		return invalid(d.ID, "runtime.api.%s: {%s} is neither captured by a request nor supplied by Crowbar", where, name)
	}
	return nil
}

// checkRequestRefs makes every event step and reply_request name a declared request.
func checkRequestRefs(d *spec.Descriptor) error {
	reqs := d.Runtime.API.Requests
	for name, e := range d.Events {
		if e.ReplyRequest != "" {
			if _, ok := reqs[e.ReplyRequest]; !ok {
				return invalid(d.ID, "events[%s].reply_request %q is not in runtime.api.requests", name, e.ReplyRequest)
			}
		}
		if d.TransportFor(name) != string(spec.ChannelAPI) {
			continue
		}
		for _, steps := range [][]spec.CallStep{e.Fresh, e.Resume, e.Action} {
			for _, s := range steps {
				if _, ok := reqs[s.Call]; !ok {
					return invalid(d.ID, "events[%s]: step call %q is not in runtime.api.requests", name, s.Call)
				}
			}
		}
	}
	return nil
}

// checkRemember makes every remember entry pick a frame and name the values a later
// event's map reads as captured.NAME.
func checkRemember(d *spec.Descriptor) error {
	for i, r := range d.Runtime.API.Remember {
		if r.In.Empty() {
			return invalid(d.ID, "runtime.api.remember[%d].in names no frame", i)
		}
		if len(r.Capture) == 0 {
			return invalid(d.ID, "runtime.api.remember[%d].capture names no value", i)
		}
		for name, path := range r.Capture {
			if !nameRE.MatchString(name) || !pathRE.MatchString(path) {
				return invalid(d.ID, "runtime.api.remember[%d].capture[%q]: needs a plain name and a path, got %q", i, name, path)
			}
		}
	}
	return nil
}
