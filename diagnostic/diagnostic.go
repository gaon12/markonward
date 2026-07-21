// Package diagnostic defines stable parser and renderer diagnostics.
package diagnostic

import "github.com/gaon12/markonward/ast"

// Severity indicates the impact of a diagnostic.
type Severity string

const (
	Info    Severity = "info"
	Warning Severity = "warning"
	Error   Severity = "error"
)

// Diagnostic summarizes a rejected or recovered Markdown construct.
type Diagnostic struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	RuleID   string   `json:"rule_id,omitempty"`
	Span     ast.Span `json:"span"`
	Recovery string   `json:"recovery,omitempty"`
	Fields   []Field  `json:"fields,omitempty"`
}

// Field carries deterministic structured context without localizing it.
type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
