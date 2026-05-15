package agent

type Finding struct {
	Line        int    `json:"line,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	CodeSnippet string `json:"code_snippet,omitempty"`
	// Quality analysis fields
	Function   string `json:"function,omitempty"`
	Complexity int    `json:"complexity,omitempty"`
	Threshold  int    `json:"threshold,omitempty"`
	Status     string `json:"status,omitempty"`
	Message    string `json:"message,omitempty"`
}

type AgentResult struct {
	Report string
	Findings []Finding
}