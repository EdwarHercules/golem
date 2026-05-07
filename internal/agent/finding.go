package agent

type Finding struct {
	Line		int		`json:"line"`
	Severity	string	`json:"severity"`
	Type		string	`json:"type"`
	Description	string	`json:"description"`
	CodeSnippet	string	`json:"code_snippet"`
}

type AgentResult struct {
	Report string
	Findings []Finding
}