package lint

import "github.com/JSLEEKR/difyctl/internal/model"

// DIFY001 — missing-app: the top-level `app` block must exist.
type ruleMissingApp struct{}

func (ruleMissingApp) ID() string { return "DIFY001" }

func (ruleMissingApp) Check(wf *model.Workflow) []Finding {
	if wf.App.Name == "" && wf.App.Mode == "" && wf.App.Description == "" {
		return []Finding{{
			Rule:     "DIFY001",
			Severity: SeverityError,
			Message:  "top-level 'app' block is empty or missing (need at least name/mode)",
			Line:     1,
		}}
	}
	return nil
}

// DIFY002 — unknown-app-mode.
type ruleUnknownAppMode struct{}

func (ruleUnknownAppMode) ID() string { return "DIFY002" }

var knownAppModes = map[string]struct{}{
	"workflow":   {},
	"chatflow":   {},
	"agent-chat": {},
	"agent_chat": {},
}

func (ruleUnknownAppMode) Check(wf *model.Workflow) []Finding {
	if wf.App.Mode == "" {
		return []Finding{{
			Rule:     "DIFY002",
			Severity: SeverityError,
			Message:  "app.mode is empty (expected 'workflow', 'chatflow', or 'agent-chat')",
		}}
	}
	if _, ok := knownAppModes[wf.App.Mode]; !ok {
		return []Finding{{
			Rule:     "DIFY002",
			Severity: SeverityError,
			Message:  "unknown app.mode '" + wf.App.Mode + "' (expected 'workflow', 'chatflow', or 'agent-chat')",
		}}
	}
	return nil
}

// DIFY003 — kind must equal "app".
type ruleKindMismatch struct{}

func (ruleKindMismatch) ID() string { return "DIFY003" }

func (ruleKindMismatch) Check(wf *model.Workflow) []Finding {
	if wf.Kind == "" {
		return []Finding{{
			Rule:     "DIFY003",
			Severity: SeverityError,
			Message:  "top-level 'kind' is missing (expected 'app')",
		}}
	}
	if wf.Kind != "app" {
		return []Finding{{
			Rule:     "DIFY003",
			Severity: SeverityError,
			Message:  "top-level 'kind' is '" + wf.Kind + "', expected 'app'",
		}}
	}
	return nil
}

// DIFY004 — missing-version.
type ruleMissingVersion struct{}

func (ruleMissingVersion) ID() string { return "DIFY004" }

func (ruleMissingVersion) Check(wf *model.Workflow) []Finding {
	if wf.Version == "" {
		return []Finding{{
			Rule:     "DIFY004",
			Severity: SeverityError,
			Message:  "top-level 'version' is missing",
		}}
	}
	return nil
}
