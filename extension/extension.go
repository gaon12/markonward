// Package extension defines deterministic parser and renderer extension hooks.
package extension

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/profile"
)

// Phase identifies the extension pipeline stage.
type Phase uint8

const (
	BlockPhase Phase = iota + 1
	InlinePhase
	TransformPhase
	RenderPhase
)

func (p Phase) String() string {
	switch p {
	case BlockPhase:
		return "block"
	case InlinePhase:
		return "inline"
	case TransformPhase:
		return "transform"
	case RenderPhase:
		return "render"
	default:
		return fmt.Sprintf("phase(%d)", p)
	}
}

// ParseContext is the restricted parser state exposed to syntax extensions.
type ParseContext interface {
	Context() context.Context
	Source() []byte
	Offset() int
	SetOffset(int)
	Builder() *ast.Builder
	Parent() ast.NodeID
	Profile() profile.Profile
}

// Match describes source consumed and the node produced by a syntax parser.
type Match struct {
	Node     ast.NodeID
	Consumed int
}

type BlockParser interface {
	ParseBlock(ParseContext) (Match, bool, error)
}

type InlineParser interface {
	ParseInline(ParseContext) (Match, bool, error)
}

type ASTTransformer interface {
	Transform(context.Context, *ast.Builder) error
}

// RenderContext lets an extension render a custom node and its children.
type RenderContext interface {
	Context() context.Context
	Writer() io.Writer
	Document() *ast.Document
	RenderChildren(ast.NodeID) error
}

type RenderHandler interface {
	Render(RenderContext, ast.NodeID, bool) error
}

// Extension registers one or more syntax, transform, or render hooks.
type Extension interface {
	Register(*Registry) error
}

// Registration describes a deterministic pipeline entry.
type Registration struct {
	ID       string
	Phase    Phase
	Priority int
	Triggers []byte
	Handler  any
}

// Registry collects and validates extension registrations without global state.
type Registry struct {
	entries []Registration
	frozen  bool
}

func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) Register(registration Registration) error {
	if r == nil {
		return fmt.Errorf("extension: nil registry")
	}
	if r.frozen {
		return fmt.Errorf("extension: registry is frozen")
	}
	registration.ID = strings.TrimSpace(registration.ID)
	if registration.ID == "" {
		return fmt.Errorf("extension: registration ID is required")
	}
	if registration.Phase < BlockPhase || registration.Phase > RenderPhase {
		return fmt.Errorf("extension: %s has invalid phase %d", registration.ID, registration.Phase)
	}
	if registration.Handler == nil {
		return fmt.Errorf("extension: %s has no handler", registration.ID)
	}
	if err := validateHandler(registration); err != nil {
		return err
	}
	for _, existing := range r.entries {
		if existing.ID == registration.ID {
			return fmt.Errorf("extension: duplicate registration ID %q", registration.ID)
		}
		if existing.Phase == registration.Phase && existing.Priority == registration.Priority && triggersOverlap(existing.Triggers, registration.Triggers) {
			return fmt.Errorf("extension: %s conflicts with %s in %s phase at priority %d", registration.ID, existing.ID, registration.Phase, registration.Priority)
		}
	}
	registration.Triggers = append([]byte(nil), registration.Triggers...)
	r.entries = append(r.entries, registration)
	return nil
}

func validateHandler(registration Registration) error {
	var valid bool
	switch registration.Phase {
	case BlockPhase:
		_, valid = registration.Handler.(BlockParser)
	case InlinePhase:
		_, valid = registration.Handler.(InlineParser)
	case TransformPhase:
		_, valid = registration.Handler.(ASTTransformer)
	case RenderPhase:
		_, valid = registration.Handler.(RenderHandler)
	}
	if !valid {
		return fmt.Errorf("extension: %s handler does not implement the %s contract", registration.ID, registration.Phase)
	}
	return nil
}

func triggersOverlap(left, right []byte) bool {
	if len(left) == 0 || len(right) == 0 {
		return true
	}
	var seen [256]bool
	for _, trigger := range left {
		seen[trigger] = true
	}
	for _, trigger := range right {
		if seen[trigger] {
			return true
		}
	}
	return false
}

// Freeze validates extenders and returns an immutable sorted Set.
func (r *Registry) Freeze(extensions ...Extension) (Set, error) {
	if r == nil {
		return Set{}, fmt.Errorf("extension: nil registry")
	}
	if r.frozen {
		return Set{}, fmt.Errorf("extension: registry is already frozen")
	}
	for index, current := range extensions {
		if current == nil {
			return Set{}, fmt.Errorf("extension: extension %d is nil", index)
		}
		if err := current.Register(r); err != nil {
			return Set{}, fmt.Errorf("extension: register extension %d: %w", index, err)
		}
	}
	r.frozen = true
	entries := append([]Registration(nil), r.entries...)
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Phase != entries[j].Phase {
			return entries[i].Phase < entries[j].Phase
		}
		if entries[i].Priority != entries[j].Priority {
			return entries[i].Priority < entries[j].Priority
		}
		return entries[i].ID < entries[j].ID
	})
	return Set{entries: entries}, nil
}

// Set is an immutable ordered collection of registrations.
type Set struct{ entries []Registration }

func (s Set) Registrations(phase Phase) []Registration {
	var result []Registration
	for _, registration := range s.entries {
		if registration.Phase == phase {
			copy := registration
			copy.Triggers = append([]byte(nil), registration.Triggers...)
			result = append(result, copy)
		}
	}
	return result
}
