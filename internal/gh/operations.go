package gh

import "fmt"

// Operation names recorded in ErrInsufficientScope warnings and ApplyResult
// skip records. internal/alter matches these strings to map skips back to
// config fields, so both ends must use these shared definitions.
const (
	OpFetchActionsPermissions            = "fetch actions permissions"
	OpFetchSelectedActionsPermissions    = "fetch selected actions permissions"
	OpSetActionsPermissions              = "set actions permissions"
	OpSetSelectedActionsPermissions      = "set selected actions permissions"
	OpFetchWorkflowPermissions           = "fetch workflow permissions"
	OpSetWorkflowPermissions             = "set workflow permissions"
	OpFetchPrivateVulnerabilityReporting = "fetch private vulnerability reporting"
	OpFetchVulnerabilityAlerts           = "fetch vulnerability alerts"
	OpFetchAutomatedSecurityFixes        = "fetch automated security fixes"
	OpPatchRepoSettings                  = "patch repo settings"
	OpSetTopics                          = "set topics"
)

// Security feature names embedded in enable/disable operation names.
const (
	FeaturePrivateVulnerabilityReporting = "private vulnerability reporting"
	FeatureVulnerabilityAlerts           = "vulnerability alerts"
	FeatureAutomatedSecurityFixes        = "automated security fixes"
)

// SecurityFeatureOp returns the operation name for enabling or disabling a
// security feature, for example "enable vulnerability alerts".
func SecurityFeatureOp(enable bool, feature string) string {
	if enable {
		return "enable " + feature
	}
	return "disable " + feature
}

// CreateLabelOp returns the operation name for creating the named label.
func CreateLabelOp(name string) string {
	return fmt.Sprintf("create label %q", name)
}

// UpdateLabelOp returns the operation name for updating the named label.
func UpdateLabelOp(name string) string {
	return fmt.Sprintf("update label %q", name)
}
