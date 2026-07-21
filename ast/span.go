package ast

import "fmt"

// Span is a half-open byte range [Start, End) in a UTF-8 source document.
type Span struct {
	Start int
	End   int
}

// NewSpan returns a validated source span.
func NewSpan(start, end int) (Span, error) {
	if start < 0 || end < start {
		return Span{}, fmt.Errorf("ast: invalid span [%d,%d)", start, end)
	}
	return Span{Start: start, End: end}, nil
}

// Len returns the number of source bytes covered by s.
func (s Span) Len() int { return s.End - s.Start }

// Empty reports whether s contains no bytes.
func (s Span) Empty() bool { return s.Start == s.End }

// Position is a human-readable source location. Line and Column are one-based;
// Offset is a zero-based byte offset.
type Position struct {
	Offset int
	Line   int
	Column int
}
