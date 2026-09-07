package gh

import "testing"

func TestPagesOperationString(t *testing.T) {
	for kind := OpFetchPagesRepository; kind <= OpGetPagesBranch; kind++ {
		if Op(kind).String() == "" {
			t.Errorf("missing description for operation %d", kind)
		}
	}
}

func TestVariableOperationString(t *testing.T) {
	for operation, want := range map[Operation]string{
		Op(OpFetchVariables):     "fetch variables",
		CreateVariableOp("NAME"): `create variable "NAME"`,
		UpdateVariableOp("NAME"): `update variable "NAME"`,
	} {
		if got := operation.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}

func TestSecurityFeatureOperationString(t *testing.T) {
	tests := []struct {
		kind    OperationKind
		feature string
	}{
		{OpSetImmutableReleases, "immutable releases"},
		{OpSetPrivateVulnerabilityReporting, "private vulnerability reporting"},
		{OpSetVulnerabilityAlerts, "vulnerability alerts"},
		{OpSetAutomatedSecurityFixes, "automated security fixes"},
	}
	for _, tt := range tests {
		t.Run(tt.feature, func(t *testing.T) {
			for _, enable := range []bool{false, true} {
				want := "disable " + tt.feature
				if enable {
					want = "enable " + tt.feature
				}
				if got := SecurityFeatureOp(enable, tt.kind).String(); got != want {
					t.Errorf("String() = %q, want %q", got, want)
				}
			}
		})
	}
}
