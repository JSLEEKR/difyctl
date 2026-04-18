package lint

import "fmt"

// Severity levels.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Finding is a single lint violation.
type Finding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// Format returns a human-readable one-line rendering.
// Example: "workflow.yml:42: [DIFY006/error] duplicate node id 'llm-1'"
func (f Finding) Format() string {
	loc := ""
	switch {
	case f.Path != "" && f.Line > 0:
		loc = fmt.Sprintf("%s:%d: ", f.Path, f.Line)
	case f.Path != "":
		loc = f.Path + ": "
	case f.Line > 0:
		loc = fmt.Sprintf("line %d: ", f.Line)
	}
	return fmt.Sprintf("%s[%s/%s] %s", loc, f.Rule, f.Severity, f.Message)
}
