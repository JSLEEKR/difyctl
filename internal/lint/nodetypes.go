package lint

// KnownNodeTypes is the canonical set of Dify workflow node types.
// We accept both hyphen and underscore forms where Dify has historically emitted both.
var KnownNodeTypes = map[string]struct{}{
	"start":               {},
	"end":                 {},
	"answer":              {},
	"llm":                 {},
	"code":                {},
	"http-request":        {},
	"http_request":        {},
	"if-else":             {},
	"if_else":             {},
	"iteration":           {},
	"iteration-start":     {},
	"iteration_start":     {},
	"knowledge-retrieval": {},
	"knowledge_retrieval": {},
	"parameter-extractor": {},
	"parameter_extractor": {},
	"question-classifier": {},
	"question_classifier": {},
	"template-transform":  {},
	"template_transform":  {},
	"variable-aggregator": {},
	"variable_aggregator": {},
	"variable-assigner":   {},
	"variable_assigner":   {},
	"tool":                {},
}

// IsKnownNodeType reports whether t is a recognized Dify workflow node type.
func IsKnownNodeType(t string) bool {
	_, ok := KnownNodeTypes[t]
	return ok
}

// IsStartType reports whether t is a Start-style node (root entry).
func IsStartType(t string) bool {
	return t == "start"
}

// IsEndType reports whether t is terminal.
func IsEndType(t string) bool {
	return t == "end" || t == "answer"
}

// IsIterationBody reports whether t is an iteration container.
func IsIterationType(t string) bool {
	return t == "iteration"
}

// IsIterationStart reports whether t is an iteration-start node.
func IsIterationStart(t string) bool {
	return t == "iteration-start" || t == "iteration_start"
}
