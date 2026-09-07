package gh

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/model"
)

type ForkPRContributorApprovalResponse struct {
	ApprovalPolicy *string `json:"approval_policy"`
}

func ReadForkPRContributorApproval(client *api.RESTClient, owner, name string) (*ForkPRContributorApprovalResponse, []error, error) {
	path := fmt.Sprintf("repos/%s/%s/actions/permissions/fork-pr-contributor-approval", owner, name)
	var current ForkPRContributorApprovalResponse
	var warnings []error
	if err := boundedHTTPError(client.Get(path, &current)); err != nil {
		err = collectAccessWarning(err, Op(OpFetchForkPRContributorApproval), "fetching fork pull request contributor approval", &warnings)
		return nil, warnings, err
	}
	if !current.known() {
		return nil, []error{forkApprovalUnknown(OpFetchForkPRContributorApproval)}, nil
	}
	return &current, nil, nil
}

func ApplyForkPRContributorApproval(client *api.RESTClient, owner, name, policy string, current *ForkPRContributorApprovalResponse) (*ApplyResult, error) {
	if !slices.Contains(model.ForkPRContributorApprovalPolicies, policy) {
		return nil, fmt.Errorf("actions.fork_pr_contributor_approval.approval_policy is invalid")
	}
	result := &ApplyResult{}
	if !current.known() {
		result.Skipped = append(result.Skipped, SkippedOperation{
			Operation: Op(OpSetForkPRContributorApproval),
			Err:       forkApprovalUnknown(OpSetForkPRContributorApproval),
		})
		return result, nil
	}
	if policy == *current.ApprovalPolicy {
		return result, nil
	}
	path := fmt.Sprintf("repos/%s/%s/actions/permissions/fork-pr-contributor-approval", owner, name)
	_, err := applyActionsWrite(client, path, map[string]any{"approval_policy": policy}, Op(OpSetForkPRContributorApproval), result)
	return result, err
}

func (r *ForkPRContributorApprovalResponse) known() bool {
	return r != nil && r.ApprovalPolicy != nil && slices.Contains(model.ForkPRContributorApprovalPolicies, *r.ApprovalPolicy)
}

func forkApprovalUnknown(kind OperationKind) *ErrInsufficientScope {
	return &ErrInsufficientScope{
		StatusCode: http.StatusOK,
		Operation:  Op(kind),
		Message:    "current fork pull request contributor approval policy is unknown or invalid",
	}
}
