package renderer

import (
	"context"
	"fmt"
	"io"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/extension"
)

// ExtensionSet is an immutable custom-node renderer lookup. A render
// registration's ID is the custom AST kind it handles.
type ExtensionSet struct {
	handlers map[string]extension.RenderHandler
}

// CompileExtensions validates extensions and collects their render handlers.
func CompileExtensions(extensions ...extension.Extension) (ExtensionSet, error) {
	registry := extension.NewRegistry()
	registrations, err := registry.Freeze(extensions...)
	if err != nil {
		return ExtensionSet{}, err
	}
	handlers := make(map[string]extension.RenderHandler)
	for _, registration := range registrations.Registrations(extension.RenderPhase) {
		handlers[registration.ID] = registration.Handler.(extension.RenderHandler)
	}
	return ExtensionSet{handlers: handlers}, nil
}

// Handler returns the handler registered for customKind.
func (s ExtensionSet) Handler(customKind string) (extension.RenderHandler, bool) {
	handler, ok := s.handlers[customKind]
	return handler, ok
}

// ExtensionContext implements extension.RenderContext for concrete renderers.
type ExtensionContext struct {
	RenderContext context.Context
	Output        io.Writer
	AST           *ast.Document
	Children      func(ast.NodeID) error
}

func (c ExtensionContext) Context() context.Context { return c.RenderContext }
func (c ExtensionContext) Writer() io.Writer        { return c.Output }
func (c ExtensionContext) Document() *ast.Document  { return c.AST }
func (c ExtensionContext) RenderChildren(id ast.NodeID) error {
	if c.Children == nil {
		return fmt.Errorf("renderer: extension child renderer is unavailable")
	}
	return c.Children(id)
}

// RenderCustom invokes a custom handler's entering and exiting callbacks. The
// handler controls child traversal by calling RenderChildren while entering.
func RenderCustom(handler extension.RenderHandler, context ExtensionContext, id ast.NodeID) error {
	if err := handler.Render(context, id, true); err != nil {
		return err
	}
	return handler.Render(context, id, false)
}
