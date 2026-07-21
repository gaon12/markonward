// Package plaintext renders AST documents without Markdown or HTML markup.
package plaintext

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gaon12/markonward/ast"
)

// Option configures a plain-text renderer.
type Option func(*Renderer)

// WithoutLinkDestinations omits the parenthesized URL after link labels.
func WithoutLinkDestinations() Option {
	return func(renderer *Renderer) { renderer.includeLinkDestination = false }
}

// Renderer produces content-oriented plain text.
type Renderer struct {
	includeLinkDestination bool
}

// New constructs a renderer that includes link destinations.
func New(options ...Option) *Renderer {
	renderer := &Renderer{includeLinkDestination: true}
	for _, option := range options {
		if option != nil {
			option(renderer)
		}
	}
	return renderer
}

// Render writes plain text to writer.
func (r *Renderer) Render(ctx context.Context, writer io.Writer, document *ast.Document) error {
	if r == nil || ctx == nil || writer == nil || document == nil {
		return fmt.Errorf("plaintext: renderer, context, writer, and document are required")
	}
	state := renderState{renderer: r, ctx: ctx, document: document}
	if err := state.children(document.Root(), 0); err != nil {
		return err
	}
	_, err := io.WriteString(writer, strings.TrimRight(state.output.String(), " \t\n")+"\n")
	return err
}

type renderState struct {
	renderer *Renderer
	ctx      context.Context
	document *ast.Document
	output   strings.Builder
}

func (s *renderState) children(parent ast.NodeID, depth int) error {
	for child := s.document.Node(parent).FirstChild(); child != ast.NoNode; child = s.document.Node(child).NextSibling() {
		if err := s.node(child, depth); err != nil {
			return err
		}
	}
	return nil
}

func (s *renderState) node(id ast.NodeID, depth int) error { //nolint:gocyclo // Explicit node behavior is easier to audit than indirect dispatch.
	if err := s.ctx.Err(); err != nil {
		return err
	}
	node := s.document.Node(id)
	switch node.Kind() {
	case ast.Invalid, ast.DocumentKind:
		return fmt.Errorf("plaintext: unexpected %s node", node.Kind())
	case ast.Paragraph:
		if err := s.children(id, depth); err != nil {
			return err
		}
		s.blankLine()
	case ast.Heading:
		if err := s.children(id, depth); err != nil {
			return err
		}
		s.blankLine()
	case ast.BlockQuote:
		if err := s.children(id, depth+1); err != nil {
			return err
		}
	case ast.List:
		if err := s.children(id, depth); err != nil {
			return err
		}
		s.newline()
	case ast.ListItem:
		parent := s.document.Node(node.Parent())
		index := 1
		for sibling := parent.FirstChild(); sibling != id && sibling != ast.NoNode; sibling = s.document.Node(sibling).NextSibling() {
			index++
		}
		s.ensureLineStart()
		s.output.WriteString(strings.Repeat("  ", depth))
		if parent.Flags()&ast.ListOrdered != 0 {
			start, _ := parent.Integers()
			if start == 0 {
				start = 1
			}
			s.output.WriteString(strconv.Itoa(start+index-1) + ". ")
		} else {
			s.output.WriteString("- ")
		}
		if err := s.children(id, depth+1); err != nil {
			return err
		}
		s.newline()
	case ast.ThematicBreak:
		s.newline()
	case ast.CodeBlock:
		s.ensureLineStart()
		s.output.WriteString(node.Text())
		s.blankLine()
	case ast.HTMLBlock, ast.RawHTML:
		// Raw tags carry no guaranteed textual semantics.
	case ast.Text, ast.CodeSpan:
		s.output.WriteString(node.Text())
	case ast.SoftBreak, ast.HardBreak:
		s.newline()
	case ast.Emphasis, ast.Strong, ast.Strikethrough:
		if err := s.children(id, depth); err != nil {
			return err
		}
	case ast.Link:
		if err := s.children(id, depth); err != nil {
			return err
		}
		if s.renderer.includeLinkDestination && node.Destination() != "" {
			s.output.WriteString(" (" + node.Destination() + ")")
		}
	case ast.Image:
		if err := s.children(id, depth); err != nil {
			return err
		}
	case ast.Autolink:
		if node.FirstChild() != ast.NoNode {
			if err := s.children(id, depth); err != nil {
				return err
			}
		} else {
			s.output.WriteString(strings.TrimPrefix(node.Destination(), "mailto:"))
		}
	case ast.Table:
		if err := s.children(id, depth); err != nil {
			return err
		}
		s.newline()
	case ast.TableHead, ast.TableBody:
		if err := s.children(id, depth); err != nil {
			return err
		}
	case ast.TableRow:
		s.ensureLineStart()
		cellIndex := 0
		for cell := node.FirstChild(); cell != ast.NoNode; cell = s.document.Node(cell).NextSibling() {
			if cellIndex > 0 {
				s.output.WriteByte('\t')
			}
			if err := s.children(cell, depth); err != nil {
				return err
			}
			cellIndex++
		}
		s.newline()
	case ast.TableCell:
		return s.children(id, depth)
	case ast.TaskCheck:
		if node.Flags()&ast.TaskChecked != 0 {
			s.output.WriteString("[x] ")
		} else {
			s.output.WriteString("[ ] ")
		}
	case ast.Custom:
		return fmt.Errorf("plaintext: no handler for custom node %q", node.CustomKind())
	default:
		return fmt.Errorf("plaintext: unsupported node kind %s", node.Kind())
	}
	return nil
}

func (s *renderState) ensureLineStart() {
	if s.output.Len() > 0 && !strings.HasSuffix(s.output.String(), "\n") {
		s.output.WriteByte('\n')
	}
}

func (s *renderState) newline() {
	if s.output.Len() == 0 || !strings.HasSuffix(s.output.String(), "\n") {
		s.output.WriteByte('\n')
	}
}

func (s *renderState) blankLine() {
	s.newline()
	if !strings.HasSuffix(s.output.String(), "\n\n") {
		s.output.WriteByte('\n')
	}
}
