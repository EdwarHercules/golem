package agent

import (
	"time"
)

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
	Report        string
	Findings      []Finding
	StepDurations []StepDuration
}

type StepDuration struct {
	StepNumber    int          `json:"step_number"`
	AttemptNumber int          `json:"attempt_number"`
	LLMCall       JSONDuration `json:"llm_call"`
	Execution     JSONDuration `json:"execution"`
	Total         JSONDuration `json:"total"`
}

type JSONDuration time.Duration

func (j JSONDuration) MarshalJSON() ([]byte, error) {
	s := time.Duration(j).String()
	return []byte(`"` + s + `"`), nil
}
