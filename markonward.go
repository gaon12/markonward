package markonward

import (
	"context"
	"fmt"
	"io"

	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
	"github.com/gaon12/markonward/renderer"
)

// Engine composes independently usable parser and renderer components.
type Engine struct {
	parser   *parser.Parser
	renderer renderer.Renderer
}

// New constructs an engine with an explicit profile and renderer.
func New(selected profile.Profile, output renderer.Renderer, options ...parser.Option) (*Engine, error) {
	if output == nil {
		return nil, fmt.Errorf("markonward: renderer is required")
	}
	p, err := parser.New(selected, options...)
	if err != nil {
		return nil, err
	}
	return &Engine{parser: p, renderer: output}, nil
}

// Parser returns the engine's immutable parser.
func (e *Engine) Parser() *parser.Parser {
	if e == nil {
		return nil
	}
	return e.parser
}

// Parse delegates to the configured parser.
func (e *Engine) Parse(ctx context.Context, source []byte) (parser.Result, error) {
	if e == nil || e.parser == nil {
		return parser.Result{}, fmt.Errorf("markonward: engine is nil")
	}
	return e.parser.Parse(ctx, source)
}

// Render writes an already parsed result with the configured renderer.
func (e *Engine) Render(ctx context.Context, writer io.Writer, result parser.Result) error {
	if e == nil || e.renderer == nil {
		return fmt.Errorf("markonward: engine is nil")
	}
	return e.renderer.Render(ctx, writer, result.Document)
}

// Convert parses source and renders it in one operation.
func (e *Engine) Convert(ctx context.Context, writer io.Writer, source []byte) (parser.Result, error) {
	result, err := e.Parse(ctx, source)
	if err != nil {
		return parser.Result{}, err
	}
	if err := e.Render(ctx, writer, result); err != nil {
		return parser.Result{}, err
	}
	return result, nil
}
