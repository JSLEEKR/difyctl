package lint

import "github.com/JSLEEKR/difyctl/internal/model"

// DIFY017 — llm-missing-model.
type ruleLLMMissingModel struct{}

func (ruleLLMMissingModel) ID() string { return "DIFY017" }

func (ruleLLMMissingModel) Check(wf *model.Workflow) []Finding {
	var out []Finding
	for _, n := range wf.Workflow.Graph.Nodes {
		if n.Type != "llm" {
			continue
		}
		if n.Data == nil {
			continue // covered by DIFY008
		}
		model, ok := n.Data["model"]
		if !ok || model == nil {
			out = append(out, Finding{
				Rule:     "DIFY017",
				Severity: SeverityError,
				Message:  "llm node '" + n.ID + "' is missing 'data.model'",
				Line:     n.Line,
			})
			continue
		}
		// Expect data.model to be a map with at least provider or name.
		m := asMap(model)
		if m == nil {
			out = append(out, Finding{
				Rule:     "DIFY017",
				Severity: SeverityError,
				Message:  "llm node '" + n.ID + "' has malformed 'data.model' (expected map)",
				Line:     n.Line,
			})
			continue
		}
		hasProvider := false
		hasName := false
		if s, _ := m["provider"].(string); s != "" {
			hasProvider = true
		}
		if s, _ := m["name"].(string); s != "" {
			hasName = true
		}
		if !hasProvider && !hasName {
			out = append(out, Finding{
				Rule:     "DIFY017",
				Severity: SeverityError,
				Message:  "llm node '" + n.ID + "' has 'data.model' without 'provider' or 'name'",
				Line:     n.Line,
			})
		}
	}
	return out
}

// DIFY018 — code-missing-code.
type ruleCodeMissingCode struct{}

func (ruleCodeMissingCode) ID() string { return "DIFY018" }

func (ruleCodeMissingCode) Check(wf *model.Workflow) []Finding {
	var out []Finding
	for _, n := range wf.Workflow.Graph.Nodes {
		if n.Type != "code" {
			continue
		}
		if n.Data == nil {
			continue
		}
		codeS, _ := n.Data["code"].(string)
		lang, _ := n.Data["code_language"].(string)
		if codeS == "" {
			out = append(out, Finding{
				Rule:     "DIFY018",
				Severity: SeverityError,
				Message:  "code node '" + n.ID + "' is missing or empty 'data.code'",
				Line:     n.Line,
			})
			continue
		}
		if lang == "" {
			out = append(out, Finding{
				Rule:     "DIFY018",
				Severity: SeverityError,
				Message:  "code node '" + n.ID + "' is missing 'data.code_language'",
				Line:     n.Line,
			})
		}
	}
	return out
}
