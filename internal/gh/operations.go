package gh

import "fmt"

// OperationKind identifies a GitHub API operation recorded in
// ErrInsufficientScope warnings and ApplyResult skip records. internal/alter
// maps skips back to config fields by kind; user-facing text comes only from
// Operation.String.
type OperationKind int

const (
	OpNone OperationKind = iota
	OpFetchActionsPermissions
	OpFetchSelectedActionsPermissions
	OpSetActionsPermissions
	OpSetSelectedActionsPermissions
	OpDisableActionsForPolicyTransition
	OpDisableActionsForPolicyUpdate
	OpFetchWorkflowPermissions
	OpSetWorkflowPermissions
	OpFetchPrivateVulnerabilityReporting
	OpFetchVulnerabilityAlerts
	OpFetchAutomatedSecurityFixes
	OpSetPrivateVulnerabilityReporting
	OpSetVulnerabilityAlerts
	OpSetAutomatedSecurityFixes
	OpPatchRepoSettings
	OpSetTopics
	OpFetchSecurityAnalysis
	OpFetchCodeScanningSetup
	OpSetCodeScanningSetup
	OpFetchCodeQualitySetup
	OpSetCodeQualitySetup
	OpCreateLabel
	OpUpdateLabel
	OpListRulesets
	OpFetchRuleset
	OpSetRuleset
	OpFetchImmutableReleases
	OpSetImmutableReleases
	OpFetchActionsRetention
	OpSetActionsRetention
	OpFetchForkPRContributorApproval
	OpSetForkPRContributorApproval
	OpFetchVariables
	OpCreateVariable
	OpUpdateVariable
	OpFetchPagesRepository
	OpFetchPages
	OpCreatePages
	OpSetPagesBuildType
	OpSetPagesDomain
	OpEnforcePagesHTTPS
	OpGetPagesEnvironment
	OpListPagesEnvironmentPolicies
	OpPutPagesEnvironment
	OpPostPagesEnvironmentPolicy
	OpGetPagesBranch
)

// Operation identifies one GitHub API operation. Enable selects the enable or
// disable description for the security feature kinds. Label carries the label
// name for OpCreateLabel and OpUpdateLabel. Variable carries the variable name.
// The zero value means no operation.
type Operation struct {
	Kind     OperationKind
	Enable   bool
	Label    string
	Variable string
}

// SkippedOperation records a sub-operation that was skipped due to
// insufficient token scope.
type SkippedOperation struct {
	Operation Operation
	Err       error // *ErrInsufficientScope
}

// ApplyResult collects the outcome of an apply operation. Skipped lists
// operations that failed with access errors and were gracefully skipped.
type ApplyResult struct {
	Skipped []SkippedOperation
	Applied []Operation // Confirmed successful writes, including before a later failure.
}

// recordAccessError appends the operation to result.Skipped when err is an
// access error, and reports whether it did. Other errors are left to the
// caller to return as hard failures.
func recordAccessError(result *ApplyResult, operation Operation, err error) bool {
	classified := classifyHTTPError(err, operation)
	if !isAccessError(classified) {
		return false
	}
	result.Skipped = append(result.Skipped, SkippedOperation{Operation: operation, Err: classified})
	return true
}

// collectAccessWarning classifies a read error. It appends an access error to
// warnings and returns nil so the read continues. Any other error is returned
// wrapped with prefix as a hard failure.
func collectAccessWarning(err error, operation Operation, prefix string, warnings *[]error) error {
	classified := classifyHTTPError(err, operation)
	if isAccessError(classified) {
		*warnings = append(*warnings, classified)
		return nil
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

// Op wraps a parameterless kind in an Operation.
func Op(kind OperationKind) Operation {
	return Operation{Kind: kind}
}

// SecurityFeatureOp returns the operation for enabling or disabling a
// security feature, for example enabling vulnerability alerts.
func SecurityFeatureOp(enable bool, kind OperationKind) Operation {
	return Operation{Kind: kind, Enable: enable}
}

// CreateLabelOp returns the operation for creating the named label.
func CreateLabelOp(name string) Operation {
	return Operation{Kind: OpCreateLabel, Label: name}
}

// UpdateLabelOp returns the operation for updating the named label.
func UpdateLabelOp(name string) Operation {
	return Operation{Kind: OpUpdateLabel, Label: name}
}

// CreateVariableOp returns the operation for creating the named variable.
func CreateVariableOp(name string) Operation {
	return Operation{Kind: OpCreateVariable, Variable: name}
}

// UpdateVariableOp returns the operation for updating the named variable.
func UpdateVariableOp(name string) Operation {
	return Operation{Kind: OpUpdateVariable, Variable: name}
}

// String returns the user-facing description of the operation, for example
// "enable vulnerability alerts".
func (o Operation) String() string {
	if text, ok := variableOperationText(o); ok {
		return text
	}
	switch o.Kind {
	case OpFetchForkPRContributorApproval:
		return "fetch fork pull request contributor approval"
	case OpSetForkPRContributorApproval:
		return "set fork pull request contributor approval"
	case OpFetchActionsRetention:
		return "fetch actions artifact and log retention"
	case OpSetActionsRetention:
		return "set actions artifact and log retention"
	case OpFetchImmutableReleases:
		return "fetch immutable releases"
	case OpFetchActionsPermissions:
		return "fetch actions permissions"
	case OpFetchSelectedActionsPermissions:
		return "fetch selected actions permissions"
	case OpSetActionsPermissions:
		return "set actions permissions"
	case OpSetSelectedActionsPermissions:
		return "set selected actions permissions"
	case OpDisableActionsForPolicyTransition:
		return "disable actions for selected policy transition"
	case OpDisableActionsForPolicyUpdate:
		return "disable actions for selected policy update"
	case OpFetchWorkflowPermissions:
		return "fetch workflow permissions"
	case OpSetWorkflowPermissions:
		return "set workflow permissions"
	case OpFetchPrivateVulnerabilityReporting:
		return "fetch private vulnerability reporting"
	case OpFetchVulnerabilityAlerts:
		return "fetch vulnerability alerts"
	case OpFetchAutomatedSecurityFixes:
		return "fetch automated security fixes"
	case OpSetImmutableReleases, OpSetPrivateVulnerabilityReporting, OpSetVulnerabilityAlerts, OpSetAutomatedSecurityFixes:
		features := map[OperationKind]string{
			OpSetImmutableReleases:             "immutable releases",
			OpSetPrivateVulnerabilityReporting: "private vulnerability reporting",
			OpSetVulnerabilityAlerts:           "vulnerability alerts",
			OpSetAutomatedSecurityFixes:        "automated security fixes",
		}
		return securityFeatureText(o.Enable, features[o.Kind])
	case OpPatchRepoSettings:
		return "patch repo settings"
	case OpSetTopics:
		return "set topics"
	case OpFetchSecurityAnalysis:
		return "fetch security and analysis"
	case OpFetchCodeScanningSetup:
		return "fetch code scanning setup"
	case OpSetCodeScanningSetup:
		return "set code scanning setup"
	case OpFetchCodeQualitySetup:
		return "fetch code quality setup"
	case OpSetCodeQualitySetup:
		return "set code quality setup"
	case OpCreateLabel, OpUpdateLabel:
		return labelOperationText(o.Kind, o.Label)
	case OpListRulesets:
		return "list rulesets"
	case OpFetchRuleset:
		return "fetch ruleset"
	case OpSetRuleset:
		return "set ruleset"
	default:
		return ""
	}
}

func pagesOperationText(kind OperationKind) (string, bool) {
	text, ok := map[OperationKind]string{
		OpFetchPagesRepository:         "fetch pages repository",
		OpFetchPages:                   "fetch pages",
		OpCreatePages:                  "create pages",
		OpSetPagesBuildType:            "set pages build type",
		OpSetPagesDomain:               "set pages domain",
		OpEnforcePagesHTTPS:            "enforce pages https",
		OpGetPagesEnvironment:          "fetch pages environment",
		OpListPagesEnvironmentPolicies: "list pages environment policies",
		OpPutPagesEnvironment:          "create pages environment",
		OpPostPagesEnvironmentPolicy:   "create pages environment branch policy",
		OpGetPagesBranch:               "fetch pages branch",
	}[kind]
	return text, ok
}

func variableOperationText(o Operation) (string, bool) {
	switch o.Kind {
	case OpFetchVariables:
		return "fetch variables", true
	case OpCreateVariable:
		return fmt.Sprintf("create variable %q", o.Variable), true
	case OpUpdateVariable:
		return fmt.Sprintf("update variable %q", o.Variable), true
	default:
		return pagesOperationText(o.Kind)
	}
}

func labelOperationText(kind OperationKind, label string) string {
	verb := "create"
	if kind == OpUpdateLabel {
		verb = "update"
	}
	return fmt.Sprintf("%s label %q", verb, label)
}

func securityFeatureText(enable bool, feature string) string {
	if enable {
		return "enable " + feature
	}
	return "disable " + feature
}
