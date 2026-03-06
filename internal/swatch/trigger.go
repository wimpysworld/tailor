package swatch

import (
	"reflect"
	"strings"
)

// TriggerCondition maps a swatch source path to a config field and the value
// that must match for the swatch to be deployed.
type TriggerCondition struct {
	ConfigField string
	Value       any
}

// triggerConditions maps source paths to their trigger conditions.
var triggerConditions = map[string]TriggerCondition{
	".github/workflows/tailor-automerge.yml": {ConfigField: "allow_auto_merge", Value: true},
}

// LookupTrigger returns the trigger condition for the given source path and
// true if one exists, or a zero value and false otherwise.
func LookupTrigger(source string) (TriggerCondition, bool) {
	tc, ok := triggerConditions[source]
	return tc, ok
}

// EvaluateTrigger returns true when the source has no trigger condition or
// when the matching field on repo satisfies the condition. It returns false
// if repo is nil and a trigger exists, or if the field value does not match.
// The repo parameter must be a pointer to a struct with yaml-tagged fields.
func EvaluateTrigger(source string, repo any) bool {
	tc, ok := LookupTrigger(source)
	if !ok {
		return true
	}
	if repo == nil || reflect.ValueOf(repo).IsNil() {
		return false
	}
	return fieldMatchesYAML(repo, tc.ConfigField, tc.Value)
}

// fieldMatchesYAML finds a struct field by its yaml tag name and compares its
// value to want. Pointer fields are dereferenced; a nil pointer never matches.
func fieldMatchesYAML(repo any, yamlTag string, want any) bool {
	rv := reflect.ValueOf(repo)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("yaml")
		name, _, _ := strings.Cut(tag, ",")
		if name != yamlTag {
			continue
		}
		fv := rv.Field(i)
		if fv.Kind() == reflect.Ptr {
			if fv.IsNil() {
				return false
			}
			fv = fv.Elem()
		}
		return reflect.DeepEqual(fv.Interface(), want)
	}
	return false
}
