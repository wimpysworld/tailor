package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// RulesetStub fakes list, read and write endpoints for ruleset 42 in acme/widget.
// Writes and LastBody record write requests, including rejected writes.
// ListQuery and ReadQuery record the last query strings.
type RulesetStub struct {
	ListStatus int
	ListBody   string
	ReadStatus int
	ReadBody   string
	// WriteStatus becomes 201 for POST when set to 200.
	WriteStatus int
	Writes      []string // "METHOD path" per write
	LastBody    map[string]any
	ListQuery   string
	ReadQuery   string
}

// NewRulesetStub defaults to 200 for reads and PUT, 201 for POST, and 404 for unknown routes.
func NewRulesetStub(listBody, readBody string) *RulesetStub {
	return &RulesetStub{
		ListStatus:  http.StatusOK,
		ListBody:    listBody,
		ReadStatus:  http.StatusOK,
		ReadBody:    readBody,
		WriteStatus: http.StatusOK,
	}
}

// Server starts a test server that serves the stub. It closes when the
// test ends.
func (s *RulesetStub) Server(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget/rulesets":
			s.ListQuery = r.URL.RawQuery
			w.WriteHeader(s.ListStatus)
			fmt.Fprint(w, s.ListBody)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget/rulesets/42":
			s.ReadQuery = r.URL.RawQuery
			w.WriteHeader(s.ReadStatus)
			fmt.Fprint(w, s.ReadBody)
		case (r.Method == http.MethodPost && r.URL.Path == "/repos/acme/widget/rulesets") ||
			(r.Method == http.MethodPut && r.URL.Path == "/repos/acme/widget/rulesets/42"):
			s.Writes = append(s.Writes, r.Method+" "+r.URL.Path)
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &s.LastBody)
			status := s.WriteStatus
			if status == http.StatusOK && r.Method == http.MethodPost {
				status = http.StatusCreated
			}
			w.WriteHeader(status)
			fmt.Fprint(w, `{"id":42}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
