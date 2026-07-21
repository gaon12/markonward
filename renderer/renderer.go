// Package renderer defines the common renderer contract.
package renderer

import (
	"context"
	"io"

	"github.com/gaon12/markonward/ast"
)

// Renderer writes one AST document without requiring the Markonward parser.
type Renderer interface {
	Render(context.Context, io.Writer, *ast.Document) error
}
