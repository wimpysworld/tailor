package gh

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
)

func variableEntry(name, value string) model.VariableEntry {
	return model.VariableEntry{Name: name, Value: &value}
}

func TestReadVariablesPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		want := fmt.Sprintf("/repos/owner/repo/actions/variables?per_page=30&page=%d", requests)
		if r.Method != http.MethodGet || r.URL.RequestURI() != want {
			t.Errorf("request = %s %s, want GET %s", r.Method, r.URL.RequestURI(), want)
		}
		if requests == 1 {
			w.Header().Set("Link", `<https://api.github.com/repos/owner/repo/actions/variables?page=2>; rel="next"`)
			fmt.Fprint(w, `{"total_count":2,"variables":[{"name":"FIRST","value":""}]}`)
			return
		}
		fmt.Fprint(w, `{"total_count":2,"variables":[{"name":"SECOND","value":" text\n{{TOKEN}} "}]}`)
	}))
	t.Cleanup(server.Close)
	got, err := ReadVariables(newTestClient(t, server), "owner", "repo")
	want := []model.VariableEntry{variableEntry("FIRST", ""), variableEntry("SECOND", " text\n{{TOKEN}} ")}
	if err != nil || !reflect.DeepEqual(got, want) || requests != 2 {
		t.Fatalf("ReadVariables = %+v, %v, requests=%d", got, err, requests)
	}
}

func TestReadVariablesEmpty(t *testing.T) {
	server := statusServer(t, http.StatusOK, `{"total_count":0,"variables":[]}`)
	got, err := ReadVariables(newTestClient(t, server), "owner", "repo")
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("ReadVariables = %#v, %v", got, err)
	}
}

func TestReadVariablesMalformed(t *testing.T) {
	for _, body := range []string{
		`null`, `{}`, `[]`, `{`, `{"total_count":0}`, `{"variables":[]}`,
		`{"total_count":null,"variables":[]}`, `{"total_count":0,"variables":null}`,
		`{"total_count":-1,"variables":[]}`, `{"total_count":1,"variables":[]}`,
		`{"total_count":1,"variables":[{}]}`, `{"total_count":1,"variables":[null]}`,
		`{"total_count":1,"variables":[{"name":"A"}]}`,
		`{"total_count":1,"variables":[{"name":"A","value":null}]}`,
		`{"total_count":1,"variables":[{"name":"A","value":false}]}`,
		`{"total_count":1,"variables":[{"name":"","value":""}]}`,
		`{"total_count":2,"variables":[{"name":"A","value":""},{"name":"a","value":""}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := statusServer(t, http.StatusOK, body)
			got, err := ReadVariables(newTestClient(t, server), "owner", "repo")
			if err == nil || got != nil {
				t.Fatalf("ReadVariables = %#v, %v, want nil and error", got, err)
			}
		})
	}
}

func TestReadVariablesFailures(t *testing.T) {
	tests := []struct {
		name            string
		status          int
		body            string
		header          string
		access, limited bool
	}{
		{"scope", 403, `{"message":"Forbidden"}`, "", true, false},
		{"missing", 404, `{"message":"Not Found"}`, "", true, false},
		{"primary limit", 403, `{"message":"Forbidden"}`, "X-RateLimit-Remaining", false, true},
		{"secondary limit", 403, `{"message":"Forbidden"}`, "Retry-After", false, true},
		{"message limit", 403, `{"message":"rate limit exceeded"}`, "", false, true},
		{"429", 429, `{"message":"Too Many Requests"}`, "", false, true},
		{"validation", 422, `{"message":"Validation Failed"}`, "", false, false},
		{"server", 500, `{"message":"failed"}`, "", false, false},
		{"malformed", 200, `{}`, "", false, false},
	}
	for _, tt := range tests {
		for _, later := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/later=%v", tt.name, later), func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if later && calls == 1 {
						w.Header().Set("Link", `<https://api.github.com/next>; rel="next"`)
						fmt.Fprint(w, `{"total_count":2,"variables":[{"name":"A","value":"a"}]}`)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					if tt.header != "" {
						w.Header().Set(tt.header, "0")
					}
					w.WriteHeader(tt.status)
					fmt.Fprint(w, tt.body)
				}))
				t.Cleanup(server.Close)
				client := newTestClient(t, server)
				got, err := ReadVariables(client, "owner", "repo")
				if err == nil || got != nil || isAccessError(err) != tt.access || isRateLimitError(err) != tt.limited {
					t.Fatalf("ReadVariables = %#v, %v", got, err)
				}
				before := calls
				if _, err := ApplyVariables(client, "owner", "repo", []model.VariableEntry{variableEntry("NEW", "value")}, got); err == nil || calls != before {
					t.Fatal("failed read permitted a write")
				}
			})
		}
	}
}

func TestApplyVariablesPayloads(t *testing.T) {
	tests := []struct {
		name             string
		desired, current []model.VariableEntry
		method, path     string
		body             map[string]string
	}{
		{"create empty", []model.VariableEntry{variableEntry("NEW", "")}, []model.VariableEntry{}, "POST", "/repos/owner/repo/actions/variables", map[string]string{"name": "NEW", "value": ""}},
		{"update empty", []model.VariableEntry{variableEntry("name", "")}, []model.VariableEntry{variableEntry("Name", "old")}, "PATCH", "/repos/owner/repo/actions/variables/Name", map[string]string{"value": ""}},
		{"exact value", []model.VariableEntry{variableEntry("NAME", " Text\n{{TOKEN}} ")}, []model.VariableEntry{variableEntry("NAME", "text")}, "PATCH", "/repos/owner/repo/actions/variables/NAME", map[string]string{"value": " Text\n{{TOKEN}} "}},
		{"escaped name", []model.VariableEntry{variableEntry("A/B", "new")}, []model.VariableEntry{variableEntry("A/B", "old")}, "PATCH", "/repos/owner/repo/actions/variables/A%2FB", map[string]string{"value": "new"}},
		{"case only", []model.VariableEntry{variableEntry("name", "same")}, []model.VariableEntry{variableEntry("NAME", "same")}, "", "", nil},
		{"undeclared", nil, []model.VariableEntry{variableEntry("OTHER", "keep")}, "", "", nil},
		{"omitted", nil, nil, "", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.Method != tt.method || r.URL.EscapedPath() != tt.path || !reflect.DeepEqual(body, tt.body) {
					t.Errorf("request = %s %s %#v", r.Method, r.URL.EscapedPath(), body)
				}
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusCreated)
					fmt.Fprint(w, `{}`)
				} else {
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			t.Cleanup(server.Close)
			result, err := ApplyVariables(newTestClient(t, server), "owner", "repo", tt.desired, tt.current)
			if err != nil || len(result.Skipped) != 0 {
				t.Fatalf("ApplyVariables = %+v, %v", result, err)
			}
			want := 1
			if tt.method == "" {
				want = 0
			}
			if calls != want {
				t.Errorf("calls = %d, want %d", calls, want)
			}
			var wantApplied []Operation
			switch tt.method {
			case http.MethodPost:
				wantApplied = []Operation{CreateVariableOp(tt.desired[0].Name)}
			case http.MethodPatch:
				wantApplied = []Operation{UpdateVariableOp(tt.desired[0].Name)}
			}
			if !reflect.DeepEqual(result.Applied, wantApplied) {
				t.Errorf("applied = %+v, want %+v", result.Applied, wantApplied)
			}
		})
	}
}

func TestApplyVariablesWriteFailures(t *testing.T) {
	tests := []struct {
		name            string
		status          int
		header, message string
		skip, limited   bool
	}{
		{"scope", 403, "", "Forbidden", true, false},
		{"404", 404, "", "Not Found", true, false},
		{"primary limit", 403, "X-RateLimit-Remaining", "Forbidden", false, true},
		{"secondary limit", 403, "Retry-After", "Forbidden", false, true},
		{"message limit", 403, "", "rate limit exceeded", false, true},
		{"429", 429, "", "Too Many Requests", false, true},
		{"validation", 422, "", "Validation Failed", false, false},
		{"server", 500, "", "failed", false, false},
	}
	for _, tt := range tests {
		for _, update := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/update=%v", tt.name, update), func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if update && r.Method != http.MethodPatch {
						t.Errorf("method = %s, want PATCH", r.Method)
					}
					if calls == 2 {
						w.Header().Set("Content-Type", "application/json")
						if tt.header != "" {
							w.Header().Set(tt.header, "0")
						}
						w.WriteHeader(tt.status)
						fmt.Fprintf(w, `{"message":%q}`, tt.message)
						return
					}
					if update {
						w.WriteHeader(http.StatusNoContent)
					} else {
						w.WriteHeader(http.StatusCreated)
						fmt.Fprint(w, `{}`)
					}
				}))
				t.Cleanup(server.Close)
				desired := []model.VariableEntry{variableEntry("A", "new"), variableEntry("SAME", "same"), variableEntry("B", "new"), variableEntry("C", "new")}
				current := []model.VariableEntry{variableEntry("SAME", "same")}
				if update {
					current = append(current, variableEntry("A", "old"), variableEntry("B", "old"), variableEntry("C", "old"))
				}
				result, err := ApplyVariables(newTestClient(t, server), "owner", "repo", desired, current)
				skip := tt.skip && (!update || tt.status != http.StatusNotFound)
				operation := CreateVariableOp
				if update {
					operation = UpdateVariableOp
				}
				wantApplied := []Operation{operation("A")}
				if skip {
					wantApplied = append(wantApplied, operation("C"))
				}
				if !reflect.DeepEqual(result.Applied, wantApplied) {
					t.Errorf("applied = %+v, want %+v", result.Applied, wantApplied)
				}
				if skip {
					if err != nil || calls != 3 || len(result.Skipped) != 1 || result.Skipped[0].Operation.Variable != "B" || !isAccessError(result.Skipped[0].Err) {
						t.Fatalf("ApplyVariables = %+v, %v, calls=%d", result, err, calls)
					}
					return
				}
				if err == nil || calls != 2 || isAccessError(err) || isRateLimitError(err) != tt.limited || !strings.Contains(err.Error(), "1 applied, 2 remaining") {
					t.Fatalf("ApplyVariables = %+v, %v, calls=%d", result, err, calls)
				}
			})
		}
	}
}

func TestVariablesTransportErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := newTestClient(t, server)
	server.Close()
	if got, err := ReadVariables(client, "owner", "repo"); got != nil || err == nil || isAccessError(err) {
		t.Fatalf("ReadVariables = %#v, %v", got, err)
	}
	if _, err := ApplyVariables(client, "owner", "repo", []model.VariableEntry{variableEntry("A", "a")}, []model.VariableEntry{}); err == nil || !strings.Contains(err.Error(), "0 applied, 1 remaining") {
		t.Fatalf("ApplyVariables error = %v", err)
	}
}

func TestVariablesBoundedErrors(t *testing.T) {
	server := statusServer(t, 422, fmt.Sprintf(`{"message":%q}`, strings.Repeat("x", 4096)))
	client := newTestClient(t, server)
	_, err := ApplyVariables(client, "owner", "repo", []model.VariableEntry{variableEntry("A", "value")}, []model.VariableEntry{})
	if err == nil || len(err.Error()) > 1024 {
		t.Fatalf("error is not bounded: %v", err)
	}
	if _, err := ReadVariables(client, "owner", "repo"); err == nil || len(err.Error()) > 1024 {
		t.Fatalf("read error is not bounded: %v", err)
	}
}

func TestApplyVariablesRetainsSkipsOnHardFailure(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"Forbidden"}`)
		case 2:
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{}`)
		default:
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"message":"Validation Failed"}`)
		}
	}))
	t.Cleanup(server.Close)
	desired := []model.VariableEntry{variableEntry("DENIED", ""), variableEntry("APPLIED", ""), variableEntry("FAILED", ""), variableEntry("PENDING", "")}
	result, err := ApplyVariables(newTestClient(t, server), "owner", "repo", desired, []model.VariableEntry{})
	if err == nil || !strings.Contains(err.Error(), "1 applied, 2 remaining") || calls != 3 {
		t.Fatalf("ApplyVariables = %+v, %v, calls=%d", result, err, calls)
	}
	if result == nil || len(result.Skipped) != 1 || result.Skipped[0].Operation.Variable != "DENIED" {
		t.Fatalf("skipped entry was lost: %+v", result)
	}
	if want := []Operation{CreateVariableOp("APPLIED")}; !reflect.DeepEqual(result.Applied, want) {
		t.Fatalf("applied = %+v, want %+v", result.Applied, want)
	}
}

func TestApplyVariablesRejectsMissingValuesBeforeWrites(t *testing.T) {
	for _, currentMissing := range []bool{false, true} {
		t.Run(fmt.Sprintf("current=%v", currentMissing), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(http.StatusCreated)
				fmt.Fprint(w, `{}`)
			}))
			t.Cleanup(server.Close)
			desired := []model.VariableEntry{variableEntry("FIRST", ""), variableEntry("INVALID", "")}
			current := []model.VariableEntry{}
			if currentMissing {
				current = append(current, model.VariableEntry{Name: "INVALID"})
			} else {
				desired[1].Value = nil
			}
			if _, err := ApplyVariables(newTestClient(t, server), "owner", "repo", desired, current); err == nil || calls != 0 {
				t.Fatalf("error = %v, calls = %d", err, calls)
			}
		})
	}
}
