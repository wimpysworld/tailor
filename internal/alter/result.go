package alter

// SkippedOperation records an operation that was skipped due to insufficient
// token scope or repository role. Name identifies the operation (e.g.
// "enable vulnerability alerts") and Reason explains why it was skipped.
type SkippedOperation struct {
	Name   string
	Reason string
}

// Result collects the outcome of an alter run across all operation
// types (settings, labels, licence). Applied lists operations that
// succeeded, Skipped lists those bypassed due to access errors, and
// Errors collects hard failures that did not abort the run.
type Result struct {
	Applied []string
	Skipped []SkippedOperation
	Errors  []error
}

// AddApplied appends one or more operation names to the applied list.
func (r *Result) AddApplied(ops ...string) {
	r.Applied = append(r.Applied, ops...)
}

// AddSkipped appends a skipped operation with the given name and reason.
func (r *Result) AddSkipped(name, reason string) {
	r.Skipped = append(r.Skipped, SkippedOperation{Name: name, Reason: reason})
}

// AddError appends a non-fatal error to the result.
func (r *Result) AddError(err error) {
	r.Errors = append(r.Errors, err)
}

// HasSkipped reports whether any operations were skipped.
func (r *Result) HasSkipped() bool {
	return len(r.Skipped) > 0
}

// HasErrors reports whether any non-fatal errors were recorded.
func (r *Result) HasErrors() bool {
	return len(r.Errors) > 0
}
