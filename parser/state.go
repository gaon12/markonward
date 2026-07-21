package parser

import (
	"context"
	"fmt"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/diagnostic"
	"github.com/gaon12/markonward/trace"
)

type reference struct {
	destination string
	title       string
}

type inlineBlock struct {
	node  ast.NodeID
	spans []ast.Span
}

type parseState struct {
	parser      *Parser
	ctx         context.Context
	done        <-chan struct{}
	source      []byte
	builder     *ast.Builder
	borrowed    bool
	diagnostics []diagnostic.Diagnostic
	references  map[string]reference
	inlines     []inlineBlock
	sequence    uint64
}

func (s *parseState) checkContext() error {
	if s.done == nil {
		return nil
	}
	select {
	case <-s.done:
		return s.ctx.Err()
	default:
		return nil
	}
}

func (s *parseState) emit(level trace.Level, event trace.Event) error {
	if !s.tracing(level) {
		return nil
	}
	s.sequence++
	event.SchemaVersion = trace.SchemaVersion
	event.Sequence = s.sequence
	event.Level = level
	if err := s.parser.traceSink.Record(event); err != nil {
		return fmt.Errorf("parser: trace sink: %w", err)
	}
	return nil
}

func (s *parseState) tracing(level trace.Level) bool {
	return s.parser.traceSink != nil && level <= s.parser.traceLevel
}

func (s *parseState) addDiagnostic(current diagnostic.Diagnostic) {
	s.diagnostics = append(s.diagnostics, current)
}
