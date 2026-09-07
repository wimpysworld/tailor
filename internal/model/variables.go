package model

// VariableEntry declares a repository Actions variable.
type VariableEntry struct {
	Name  string  `yaml:"name"`
	Value *string `yaml:"value"`
}
