package model

// VariableEntry declares a repository Actions variable.
// Value distinguishes a missing value from an explicitly empty string during validation.
type VariableEntry struct {
	Name  string  `yaml:"name"`
	Value *string `yaml:"value"`
}
