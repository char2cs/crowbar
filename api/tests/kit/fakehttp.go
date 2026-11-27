package kit

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type RecordedRequest struct {
	Method      string
	Path        string
	Body        string
	HasAuth     bool
	LastEventID string
}

type fakeRoute struct {
	status int
	body   string
}

type FakeHTTP struct {
	URL string

	mu         sync.Mutex
	routes     map[string]fakeRoute
	streamPath string
	requests   []RecordedRequest

	events chan string
	drops  chan struct{}
	opened chan struct{}
}

func NewFakeHTTP(t *testing.T) *FakeHTTP {
	t.Helper()
	f := &FakeHTTP{
		routes: map[string]fakeRoute{},
		events: make(chan string, 64),
		drops:  make(chan struct{}, 8),
		opened: make(chan struct{}, 8),
	}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

func (f *FakeHTTP) Route(method, path string, status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[method+" "+path] = fakeRoute{status, body}
}

func (f *FakeHTTP) StreamAt(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.streamPath = path
}

func (f *FakeHTTP) Emit(raw string) { f.events <- raw }

func (f *FakeHTTP) Event(id, name, data string) {
	f.Emit(fmt.Sprintf("id: %s\nevent: %s\ndata: %s\n\n", id, name, data))
}

func (f *FakeHTTP) DropStream() { f.drops <- struct{}{} }

func (f *FakeHTTP) StreamOpened() <-chan struct{} { return f.opened }

func (f *FakeHTTP) Requests() []RecordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]RecordedRequest(nil), f.requests...)
}

func (f *FakeHTTP) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, RecordedRequest{
		Method: r.Method, Path: r.URL.Path, Body: string(body),
		HasAuth: r.Header.Get("Authorization") != "", LastEventID: r.Header.Get("Last-Event-ID"),
	})
	route, routed := f.routes[r.Method+" "+r.URL.Path]
	isStream := r.Method == http.MethodGet && r.URL.Path == f.streamPath
	f.mu.Unlock()
	if isStream {
		f.stream(w, r)
		return
	}
	if !routed {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(route.status)
	if _, err := io.WriteString(w, route.body); err != nil {
		log.Printf("fakehttp: write: %v", err)
	}
}

func (f *FakeHTTP) stream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flush := http.NewResponseController(w).Flush
	if flush() != nil {
		return
	}
	f.opened <- struct{}{}
	for {
		select {
		case raw := <-f.events:
			if _, err := io.WriteString(w, raw); err != nil || flush() != nil {
				return
			}
		case <-f.drops:
			return
		case <-r.Context().Done():
			return
		}
	}
}
