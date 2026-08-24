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
	OpCreateLabel
	OpUpdateLabel
)

// Operation identifies one GitHub API operation. Enable selects the enable or
// disable description for the security feature kinds. Label carries the label
// name for OpCreateLabel and OpUpdateLabel. The zero value means no operation.
type Operation struct {
	Kind   OperationKind
	Enable bool
	Label  string
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

// String returns the user-facing description of the operation, for example
// "enable vulnerability alerts".
func (o Operation) String() string {
	switch o.Kind {
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
	case OpSetPrivateVulnerabilityReporting:
		return securityFeatureText(o.Enable, "private vulnerability reporting")
	case OpSetVulnerabilityAlerts:
		return securityFeatureText(o.Enable, "vulnerability alerts")
	case OpSetAutomatedSecurityFixes:
		return securityFeatureText(o.Enable, "automated security fixes")
	case OpPatchRepoSettings:
		return "patch repo settings"
	case OpSetTopics:
		return "set topics"
	case OpCreateLabel:
		return fmt.Sprintf("create label %q", o.Label)
	case OpUpdateLabel:
		return fmt.Sprintf("update label %q", o.Label)
	default:
		return ""
	}
}

func securityFeatureText(enable bool, feature string) string {
	if enable {
		return "enable " + feature
	}
	return "disable " + feature
}
