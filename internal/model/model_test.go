package model

import (
	"testing"
)

func TestLabelNeedsUpdateCasingDiffers(t *testing.T) {
	existing := LabelEntry{Name: "bug", Color: "d73a4a", Description: "desc"}
	desired := LabelEntry{Name: "Bug", Color: "d73a4a", Description: "desc"}
	if !LabelNeedsUpdate(existing, desired) {
		t.Error("LabelNeedsUpdate() = false, want true when name casing differs")
	}
}

func TestLabelNeedsUpdateExactMatch(t *testing.T) {
	existing := LabelEntry{Name: "bug", Color: "d73a4a", Description: "desc"}
	desired := LabelEntry{Name: "bug", Color: "d73a4a", Description: "desc"}
	if LabelNeedsUpdate(existing, desired) {
		t.Error("LabelNeedsUpdate() = true, want false when labels are identical")
	}
}

func TestLabelNeedsUpdateColourDiffers(t *testing.T) {
	existing := LabelEntry{Name: "bug", Color: "d73a4a", Description: "desc"}
	desired := LabelEntry{Name: "bug", Color: "ff0000", Description: "desc"}
	if !LabelNeedsUpdate(existing, desired) {
		t.Error("LabelNeedsUpdate() = false, want true when colour differs")
	}
}

func TestLabelNeedsUpdateDescriptionDiffers(t *testing.T) {
	existing := LabelEntry{Name: "bug", Color: "d73a4a", Description: "old"}
	desired := LabelEntry{Name: "bug", Color: "d73a4a", Description: "new"}
	if !LabelNeedsUpdate(existing, desired) {
		t.Error("LabelNeedsUpdate() = false, want true when description differs")
	}
}

func TestLabelNeedsUpdateColourCaseInsensitive(t *testing.T) {
	existing := LabelEntry{Name: "bug", Color: "D73A4A", Description: "desc"}
	desired := LabelEntry{Name: "bug", Color: "d73a4a", Description: "desc"}
	if LabelNeedsUpdate(existing, desired) {
		t.Error("LabelNeedsUpdate() = true, want false when colour differs only in casing")
	}
}

func TestRepositorySettingFieldsMetadata(t *testing.T) {
	fields := RepositorySettingFields(nil)

	wantKeys := []string{
		"description",
		"homepage",
		"has_wiki",
		"has_discussions",
		"has_projects",
		"has_issues",
		"allow_merge_commit",
		"allow_squash_merge",
		"allow_rebase_merge",
		"squash_merge_commit_title",
		"squash_merge_commit_message",
		"merge_commit_title",
		"merge_commit_message",
		"delete_branch_on_merge",
		"allow_update_branch",
		"allow_auto_merge",
		"web_commit_signoff_required",
		"private_vulnerability_reporting_enabled",
		"vulnerability_alerts_enabled",
		"automated_security_fixes_enabled",
		"topics",
		"default_workflow_permissions",
		"can_approve_pull_request_reviews",
	}

	if len(fields) != len(wantKeys) {
		t.Fatalf("got %d fields, want %d", len(fields), len(wantKeys))
	}
	for i, want := range wantKeys {
		if fields[i].YAMLKey != want {
			t.Errorf("field %d YAMLKey = %q, want %q", i, fields[i].YAMLKey, want)
		}
		if fields[i].Index != i {
			t.Errorf("field %d Index = %d, want %d", i, fields[i].Index, i)
		}
		if fields[i].Set {
			t.Errorf("field %s Set = true for nil settings", fields[i].YAMLKey)
		}
	}
}

func TestActionsSettingFieldsMetadata(t *testing.T) {
	fields := ActionsSettingFields(nil)

	wantKeys := []string{
		"enabled",
		"allowed_actions",
		"sha_pinning_required",
		"github_owned_allowed",
		"verified_allowed",
		"patterns_allowed",
	}

	if len(fields) != len(wantKeys) {
		t.Fatalf("got %d fields, want %d", len(fields), len(wantKeys))
	}
	for i, want := range wantKeys {
		if fields[i].YAMLKey != want {
			t.Errorf("field %d YAMLKey = %q, want %q", i, fields[i].YAMLKey, want)
		}
		if fields[i].Set {
			t.Errorf("field %s Set = true for nil settings", fields[i].YAMLKey)
		}
	}
}

func TestRepositorySettingFieldsValues(t *testing.T) {
	topics := []string{"go", "cli"}
	settings := &RepositorySettings{
		Description: new("test"),
		HasWiki:     new(false),
		Topics:      &topics,
	}

	fields := RepositorySettingFields(settings)
	seen := map[string]RepositorySettingField{}
	for _, field := range fields {
		seen[field.YAMLKey] = field
	}

	for _, key := range []string{"description", "has_wiki", "topics"} {
		field := seen[key]
		if !field.Set {
			t.Errorf("%s Set = false, want true", key)
		}
		if !field.Value.IsValid() || field.Value.IsNil() {
			t.Errorf("%s Value is invalid or nil", key)
		}
	}

	if got := seen["description"].Value.Elem().String(); got != "test" {
		t.Errorf("description value = %q, want %q", got, "test")
	}
	if got := seen["has_wiki"].Value.Elem().Bool(); got {
		t.Error("has_wiki value = true, want false")
	}
	if got := seen["topics"].Value.Elem().Len(); got != 2 {
		t.Errorf("topics length = %d, want 2", got)
	}

	if seen["homepage"].Set {
		t.Error("homepage Set = true, want false")
	}
}
