package gh

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
)

// PagesRepositoryState holds repository metadata and proved token write access.
type PagesRepositoryState struct {
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	Homepage      string `json:"homepage"`
	WriteAccess   bool   `json:"-"`
	AdminAccess   bool   `json:"-"`
}

// PagesState contains only the site fields used by Pages reconciliation.
type PagesState struct {
	Exists               bool              `json:"-"`
	BuildType            string            `json:"build_type"`
	CNAME                string            `json:"cname"`
	HTMLURL              string            `json:"html_url"`
	HTTPSEnforced        bool              `json:"https_enforced"`
	ProtectedDomainState string            `json:"protected_domain_state"`
	HTTPSCertificate     *PagesCertificate `json:"https_certificate"`
}

// PagesCertificate reports the certificate state and its covered domains.
type PagesCertificate struct {
	State   string   `json:"state"`
	Domains []string `json:"domains"`
}

// ErrPagesPending means that DNS, ownership or certificate setup needs another run.
type ErrPagesPending struct{ Reason string }

func (e *ErrPagesPending) Error() string { return "pages: " + e.Reason }

// ReadPagesRepository proves classic token grants separately from the repository role.
func ReadPagesRepository(client *api.RESTClient, owner, name string) (*PagesRepositoryState, error) {
	response, err := client.Request(http.MethodGet, fmt.Sprintf("repos/%s/%s", owner, name), nil)
	if err != nil {
		return nil, pagesError(err, OpFetchPagesRepository)
	}
	defer response.Body.Close()
	var repo struct {
		PagesRepositoryState
		Permissions struct {
			Admin    bool `json:"admin"`
			Maintain bool `json:"maintain"`
		} `json:"permissions"`
	}
	if err := json.NewDecoder(response.Body).Decode(&repo); err != nil {
		return nil, fmt.Errorf("decode pages repository: %w", err)
	}
	granted := slices.Contains(parseCSVScopes(response.Header.Get("X-OAuth-Scopes")), "repo")
	repo.WriteAccess = granted && (repo.Permissions.Admin || repo.Permissions.Maintain)
	repo.AdminAccess = granted && repo.Permissions.Admin
	if repo.Private {
		return &repo.PagesRepositoryState, &ErrSetupSkipped{Reason: SetupNotAvailable, Operation: Op(OpFetchPagesRepository)}
	}
	if !repo.WriteAccess {
		return &repo.PagesRepositoryState, &ErrInsufficientScope{Operation: Op(OpFetchPagesRepository), Message: "pages write access is not proved by granted token scopes and repository role"}
	}
	if repo.DefaultBranch == "" {
		return nil, fmt.Errorf("pages repository response is missing default_branch")
	}
	return &repo.PagesRepositoryState, nil
}

func pagesPath(owner, name string) string { return fmt.Sprintf("repos/%s/%s/pages", owner, name) }

// ReadPages treats a 404 as absence only after the caller proves write access.
func ReadPages(client *api.RESTClient, owner, name string, accessProven bool) (*PagesState, error) {
	var state PagesState
	err := client.Get(pagesPath(owner, name), &state)
	if err != nil {
		var httpErr *api.HTTPError
		if accessProven && errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return &PagesState{}, nil
		}
		return nil, pagesError(err, OpFetchPages)
	}
	state.Exists = true
	return &state, nil
}

// ApplyPages reconciles only the Pages endpoint. The supplied state comes from ReadPages.
// A nil cname preserves the domain. An empty cname removes the domain.
func ApplyPages(client *api.RESTClient, owner, name string, state *PagesState, cname *string) (ApplyResult, *PagesState, error) {
	var result ApplyResult
	if state == nil {
		return result, nil, fmt.Errorf("pages state is required")
	}
	current := *state
	if err := ensurePagesWorkflow(client, owner, name, &current, &result); err != nil {
		return result, &current, err
	}
	if cname != nil && *cname != current.CNAME {
		var domain any
		if *cname != "" {
			domain = *cname
		}
		err := writePages(client, owner, name, http.MethodPut, map[string]any{"cname": domain}, http.StatusNoContent, OpSetPagesDomain)
		if err != nil {
			return result, &current, pagesDomainError(err)
		}
		result.Applied = append(result.Applied, Op(OpSetPagesDomain))
		current.CNAME = *cname
		current.HTTPSEnforced = false
	}
	effective, err := ReadPages(client, owner, name, false)
	if err != nil {
		return result, &current, err
	}
	if err := ensurePagesHTTPS(client, owner, name, effective, &result); err != nil {
		return result, effective, err
	}
	return result, effective, nil
}

func ensurePagesWorkflow(client *api.RESTClient, owner, name string, state *PagesState, result *ApplyResult) error {
	if !state.Exists {
		err := writePages(client, owner, name, http.MethodPost, map[string]any{"build_type": "workflow"}, http.StatusCreated, OpCreatePages)
		if err != nil {
			if !pagesCreationConflict(err) {
				return err
			}
			current, readErr := ReadPages(client, owner, name, false)
			if readErr != nil {
				return fmt.Errorf("re-read pages after creation conflict: %w", readErr)
			}
			*state = *current
		} else {
			result.Applied = append(result.Applied, Op(OpCreatePages))
			state.Exists, state.BuildType = true, "workflow"
		}
	}
	if state.BuildType == "workflow" {
		return nil
	}
	err := writePages(client, owner, name, http.MethodPut, map[string]any{"build_type": "workflow"}, http.StatusNoContent, OpSetPagesBuildType)
	if err != nil {
		return err
	}
	result.Applied = append(result.Applied, Op(OpSetPagesBuildType))
	state.BuildType = "workflow"
	return nil
}

func pagesCreationConflict(err error) bool {
	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	return httpErr.StatusCode == http.StatusConflict || (httpErr.StatusCode == http.StatusUnprocessableEntity && strings.Contains(strings.ToLower(httpErr.Message), "already exists"))
}

func ensurePagesHTTPS(client *api.RESTClient, owner, name string, state *PagesState, result *ApplyResult) error {
	if state.HTTPSEnforced {
		return nil
	}
	if state.CNAME != "" && state.ProtectedDomainState != "" && state.ProtectedDomainState != "verified" {
		return &ErrPagesPending{Reason: "domain ownership verification pending"}
	}
	domain := state.CNAME
	if domain == "" {
		parsed, err := url.Parse(state.HTMLURL)
		if err != nil {
			return fmt.Errorf("parse pages url: %w", err)
		}
		domain = parsed.Hostname()
	}
	certificate := state.HTTPSCertificate
	if certificate == nil || (certificate.State != "approved" && certificate.State != "uploaded") || !pagesCertificateCovers(certificate, domain) {
		return &ErrPagesPending{Reason: "https certificate pending"}
	}
	err := writePages(client, owner, name, http.MethodPut, map[string]any{"https_enforced": true}, http.StatusNoContent, OpEnforcePagesHTTPS)
	if err != nil {
		return pagesDomainError(err)
	}
	result.Applied = append(result.Applied, Op(OpEnforcePagesHTTPS))
	state.HTTPSEnforced = true
	return nil
}

func pagesCertificateCovers(certificate *PagesCertificate, domain string) bool {
	if domain == "" {
		return false
	}
	for _, covered := range certificate.Domains {
		if strings.EqualFold(covered, domain) {
			return true
		}
		if strings.HasPrefix(covered, "*.") {
			_, suffix, found := strings.Cut(domain, ".")
			if found && strings.EqualFold(suffix, covered[2:]) {
				return true
			}
		}
	}
	return false
}

func pagesDomainError(err error) error {
	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnprocessableEntity {
		return err
	}
	message := strings.ToLower(httpErr.Message)
	for _, phrase := range []string{"certificate is not yet available", "certificate has not been issued", "domain does not resolve", "domain is not verified", "domain verification is required"} {
		if strings.Contains(message, phrase) {
			return &ErrPagesPending{Reason: boundedSanitisedText(httpErr.Message, 240)}
		}
	}
	return err
}

func writePages(client *api.RESTClient, owner, name, method string, body map[string]any, status int, operation OperationKind) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	response, err := client.Request(method, pagesPath(owner, name), bytes.NewReader(payload))
	if err != nil {
		return pagesError(err, operation)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		return fmt.Errorf("%s: unexpected HTTP status %d", Op(operation), response.StatusCode)
	}
	return nil
}

func pagesError(err error, operation OperationKind) error {
	return fmt.Errorf("%s: %w", Op(operation), classifyHTTPError(boundedHTTPError(err), Op(operation)))
}
