package lint

// DefaultRules returns the full rule catalog in stable order.
func DefaultRules() []Rule {
	return []Rule{
		ruleMissingApp{},            // DIFY001
		ruleUnknownAppMode{},        // DIFY002
		ruleKindMismatch{},          // DIFY003
		ruleMissingVersion{},        // DIFY004
		ruleMissingNodeID{},         // DIFY005
		ruleDuplicateNodeID{},       // DIFY006
		ruleUnknownNodeType{},       // DIFY007
		ruleMissingNodeData{},       // DIFY008
		ruleEdgeDanglingSource{},    // DIFY009
		ruleEdgeDanglingTarget{},    // DIFY010
		ruleDuplicateEdge{},         // DIFY011
		ruleOrphanNode{},            // DIFY012
		ruleUnresolvedVarRef{},      // DIFY013
		ruleGraphCycle{},            // DIFY014
		ruleMissingStart{},          // DIFY015
		ruleMissingEnd{},            // DIFY016
		ruleLLMMissingModel{},       // DIFY017
		ruleCodeMissingCode{},       // DIFY018
		ruleIterationMissingStart{}, // DIFY019
		ruleUnreachableFromStart{},  // DIFY020
	}
}

// RuleIDs returns the IDs of all built-in rules in catalog order.
func RuleIDs() []string {
	rs := DefaultRules()
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID()
	}
	return out
}
