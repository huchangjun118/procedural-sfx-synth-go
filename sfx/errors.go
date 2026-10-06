package sfx

import "fmt"

// ValidationError pinpoints a single invalid parameter.
type ValidationError struct {
	Field  string // dotted path, e.g. "Filter.CutoffHz"
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("sfx: invalid parameter %s: %s", e.Field, e.Reason)
}

// BakeError wraps a failure of a single bake task.
type BakeError struct {
	TaskID string
	Err    error
}

func (e *BakeError) Error() string {
	return fmt.Sprintf("sfx: bake task %q failed: %v", e.TaskID, e.Err)
}

func (e *BakeError) Unwrap() error { return e.Err }
