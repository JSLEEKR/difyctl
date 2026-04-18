// Package lint is the rule engine and built-in rule catalog for difyctl.
//
// Rules are small, stateless, and registered in rules.go. Each rule has a
// stable ID (DIFY001..DIFY0NN) that can be referenced in suppression comments
// or CI config.
package lint

import (
	"sort"

	"github.com/JSLEEKR/difyctl/internal/model"
)

// Rule is the interface every built-in lint rule implements.
type Rule interface {
	ID() string
	Check(wf *model.Workflow) []Finding
}

// Run executes the provided rules against the workflow and returns findings
// sorted by (Rule, Line, Message) for deterministic output.
func Run(rules []Rule, wf *model.Workflow) []Finding {
	var all []Finding
	for _, r := range rules {
		fs := r.Check(wf)
		for i := range fs {
			if fs[i].Path == "" {
				fs[i].Path = wf.Path
			}
		}
		all = append(all, fs...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Rule != all[j].Rule {
			return all[i].Rule < all[j].Rule
		}
		if all[i].Line != all[j].Line {
			return all[i].Line < all[j].Line
		}
		return all[i].Message < all[j].Message
	})
	return all
}

// CountBySeverity counts findings grouped by severity.
func CountBySeverity(fs []Finding) map[string]int {
	out := map[string]int{SeverityError: 0, SeverityWarning: 0}
	for _, f := range fs {
		out[f.Severity]++
	}
	return out
}

// HasErrors reports whether any finding has severity=error.
func HasErrors(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}
