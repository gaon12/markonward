// Package html renders Markonward AST documents as safe HTML.
package html

import (
	"context"
	"fmt"
	stdhtml "html"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/gaon12/markonward/ast"
)

// Option configures an HTML Renderer.
type Option func(*Renderer)

// WithUnsafe allows raw HTML and otherwise-dangerous URL schemes. GFM's
// tagfilter remains active for documents parsed with a GFM-derived profile.
func WithUnsafe() Option {
	return func(renderer *Renderer) { renderer.unsafe = true }
}

// WithXHTML emits XHTML-compatible void elements.
func WithXHTML() Option {
	return func(renderer *Renderer) { renderer.xhtml = true }
}

// Renderer is immutable after construction and safe for concurrent use.
type Renderer struct {
	unsafe bool
	xhtml  bool
}

// New constructs a safe HTML renderer.
func New(options ...Option) *Renderer {
	renderer := &Renderer{}
	for _, option := range options {
		if option != nil {
			option(renderer)
		}
	}
	return renderer
}

// Render writes document to writer.
func (r *Renderer) Render(ctx context.Context, writer io.Writer, document *ast.Document) error {
	if r == nil {
		return fmt.Errorf("html: renderer is nil")
	}
	if ctx == nil {
		return fmt.Errorf("html: context is nil")
	}
	if writer == nil {
		return fmt.Errorf("html: writer is nil")
	}
	if document == nil || document.Root() == ast.NoNode {
		return fmt.Errorf("html: document is nil or empty")
	}
	state := renderState{renderer: r, ctx: ctx, writer: writer, document: document}
	return state.children(document.Root(), renderContext{})
}

type renderContext struct {
	inTightList bool
}

type renderState struct {
	renderer *Renderer
	ctx      context.Context
	writer   io.Writer
	document *ast.Document
}

func (s *renderState) children(parent ast.NodeID, context renderContext) error {
	for child := s.document.Node(parent).FirstChild(); child != ast.NoNode; child = s.document.Node(child).NextSibling() {
		if err := s.node(child, context); err != nil {
			return err
		}
	}
	return nil
}

func (s *renderState) node(id ast.NodeID, context renderContext) error { //nolint:gocyclo // Node kinds form a deliberately explicit rendering table.
	if err := s.ctx.Err(); err != nil {
		return err
	}
	node := s.document.Node(id)
	switch node.Kind() {
	case ast.Invalid, ast.DocumentKind:
		return fmt.Errorf("html: unexpected %s node %d", node.Kind(), id)
	case ast.Paragraph:
		if context.inTightList {
			return s.children(id, context)
		}
		return s.container(id, "<p>", "</p>\n", context)
	case ast.Heading:
		level, _ := node.Integers()
		if level < 1 || level > 6 {
			return fmt.Errorf("html: invalid heading level %d", level)
		}
		tag := "h" + strconv.Itoa(level)
		return s.container(id, "<"+tag+">", "</"+tag+">\n", context)
	case ast.BlockQuote:
		return s.container(id, "<blockquote>\n", "</blockquote>\n", context)
	case ast.List:
		ordered := node.Flags()&ast.ListOrdered != 0
		open, close := "<ul>\n", "</ul>\n"
		if ordered {
			start, _ := node.Integers()
			open = "<ol"
			if start != 1 && start != 0 {
				open += ` start="` + strconv.Itoa(start) + `"`
			}
			open += ">\n"
			close = "</ol>\n"
		}
		return s.container(id, open, close, renderContext{inTightList: listIsTight(s.document, id)})
	case ast.ListItem:
		return s.container(id, "<li>", "</li>\n", context)
	case ast.ThematicBreak:
		return s.write(s.void("hr"))
	case ast.CodeBlock:
		info := strings.Fields(node.Title())
		if err := s.write("<pre><code"); err != nil {
			return err
		}
		if len(info) > 0 {
			if err := s.write(` class="language-` + stdhtml.EscapeString(info[0]) + `"`); err != nil {
				return err
			}
		}
		return s.write(">" + stdhtml.EscapeString(node.Text()) + "</code></pre>\n")
	case ast.HTMLBlock, ast.RawHTML:
		return s.rawHTML(node.Text(), node.Kind() == ast.HTMLBlock)
	case ast.Text:
		return s.write(stdhtml.EscapeString(node.Text()))
	case ast.SoftBreak:
		return s.write("\n")
	case ast.HardBreak:
		return s.write(s.void("br") + "\n")
	case ast.CodeSpan:
		return s.write("<code>" + stdhtml.EscapeString(node.Text()) + "</code>")
	case ast.Emphasis:
		return s.container(id, "<em>", "</em>", context)
	case ast.Strong:
		return s.container(id, "<strong>", "</strong>", context)
	case ast.Strikethrough:
		return s.container(id, "<del>", "</del>", context)
	case ast.Link:
		return s.link(id, false, context)
	case ast.Image:
		return s.link(id, true, context)
	case ast.Autolink:
		destination := strings.TrimPrefix(node.Destination(), "mailto:")
		return s.anchor(id, destination, context)
	case ast.Table:
		return s.container(id, "<table>\n", "</table>\n", context)
	case ast.TableHead:
		return s.container(id, "<thead>\n", "</thead>\n", context)
	case ast.TableBody:
		return s.container(id, "<tbody>\n", "</tbody>\n", context)
	case ast.TableRow:
		return s.container(id, "<tr>\n", "</tr>\n", context)
	case ast.TableCell:
		tag := "td"
		if s.document.Node(node.Parent()).Kind() == ast.TableRow && s.document.Node(s.document.Node(node.Parent()).Parent()).Kind() == ast.TableHead {
			tag = "th"
		}
		alignment := alignmentStyle(node.Flags())
		return s.container(id, "<"+tag+alignment+">", "</"+tag+">\n", context)
	case ast.TaskCheck:
		checked := ""
		if node.Flags()&ast.TaskChecked != 0 {
			checked = ` checked=""`
		}
		if s.renderer.xhtml {
			return s.write(`<input disabled="" type="checkbox"` + checked + " /> ")
		}
		return s.write(`<input disabled="" type="checkbox"` + checked + "> ")
	case ast.Custom:
		return fmt.Errorf("html: no handler for custom node %q", node.CustomKind())
	}
	return fmt.Errorf("html: unsupported node kind %s", node.Kind())
}

func (s *renderState) container(id ast.NodeID, open, close string, context renderContext) error {
	if err := s.write(open); err != nil {
		return err
	}
	if err := s.children(id, context); err != nil {
		return err
	}
	return s.write(close)
}

func (s *renderState) link(id ast.NodeID, image bool, context renderContext) error {
	node := s.document.Node(id)
	if image {
		alt := collectText(s.document, id)
		if err := s.write(`<img src="` + s.safeURL(node.Destination()) + `" alt="` + stdhtml.EscapeString(alt) + `"`); err != nil {
			return err
		}
		if node.Title() != "" {
			if err := s.write(` title="` + stdhtml.EscapeString(node.Title()) + `"`); err != nil {
				return err
			}
		}
		if s.renderer.xhtml {
			return s.write(" />")
		}
		return s.write(">")
	}
	if err := s.write(`<a href="` + s.safeURL(node.Destination()) + `"`); err != nil {
		return err
	}
	if node.Title() != "" {
		if err := s.write(` title="` + stdhtml.EscapeString(node.Title()) + `"`); err != nil {
			return err
		}
	}
	if err := s.write(">"); err != nil {
		return err
	}
	if err := s.children(id, context); err != nil {
		return err
	}
	return s.write("</a>")
}

func (s *renderState) anchor(id ast.NodeID, label string, context renderContext) error {
	node := s.document.Node(id)
	if err := s.write(`<a href="` + s.safeURL(node.Destination()) + `">`); err != nil {
		return err
	}
	if node.FirstChild() != ast.NoNode {
		if err := s.children(id, context); err != nil {
			return err
		}
	} else if err := s.write(stdhtml.EscapeString(label)); err != nil {
		return err
	}
	return s.write("</a>")
}

func (s *renderState) rawHTML(raw string, block bool) error {
	if !s.renderer.unsafe {
		raw = stdhtml.EscapeString(raw)
	} else if profileNeedsTagFilter(s.document.Profile()) {
		raw = applyTagFilter(raw)
	}
	if block && !strings.HasSuffix(raw, "\n") {
		raw += "\n"
	}
	return s.write(raw)
}

func (s *renderState) safeURL(destination string) string {
	if !s.renderer.unsafe && dangerousURL(destination) {
		return ""
	}
	return stdhtml.EscapeString(destination)
}

func (s *renderState) void(tag string) string {
	if s.renderer.xhtml {
		return "<" + tag + " />"
	}
	return "<" + tag + ">"
}

func (s *renderState) write(value string) error {
	_, err := io.WriteString(s.writer, value)
	return err
}

func listIsTight(document *ast.Document, list ast.NodeID) bool {
	for item := document.Node(list).FirstChild(); item != ast.NoNode; item = document.Node(item).NextSibling() {
		paragraphs := 0
		for child := document.Node(item).FirstChild(); child != ast.NoNode; child = document.Node(child).NextSibling() {
			if document.Node(child).Kind() == ast.Paragraph {
				paragraphs++
			}
		}
		if paragraphs > 1 {
			return false
		}
	}
	return true
}

func alignmentStyle(flags uint32) string {
	switch {
	case flags&ast.TableAlignCenter != 0:
		return ` align="center"`
	case flags&ast.TableAlignLeft != 0:
		return ` align="left"`
	case flags&ast.TableAlignRight != 0:
		return ` align="right"`
	default:
		return ""
	}
}

func collectText(document *ast.Document, parent ast.NodeID) string {
	var builder strings.Builder
	_ = document.Walk(parent, func(node ast.Node, entering bool) error {
		if entering && (node.Kind() == ast.Text || node.Kind() == ast.CodeSpan) {
			builder.WriteString(node.Text())
		}
		return nil
	})
	return builder.String()
}

func dangerousURL(destination string) bool {
	normalized := strings.Map(func(current rune) rune {
		if unicode.IsSpace(current) || unicode.IsControl(current) {
			return -1
		}
		return unicode.ToLower(current)
	}, destination)
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme == "" {
		return false
	}
	switch parsed.Scheme {
	case "http", "https", "mailto", "tel", "ftp":
		return false
	default:
		return true
	}
}

func profileNeedsTagFilter(profileID string) bool {
	return strings.HasPrefix(profileID, "gfm-") || strings.HasPrefix(profileID, "enhancemark-")
}

var disallowedTags = map[string]struct{}{
	"title": {}, "textarea": {}, "style": {}, "xmp": {}, "iframe": {}, "noembed": {}, "noframes": {}, "script": {}, "plaintext": {},
}

func applyTagFilter(raw string) string {
	var output strings.Builder
	output.Grow(len(raw))
	for position := 0; position < len(raw); {
		if raw[position] != '<' {
			output.WriteByte(raw[position])
			position++
			continue
		}
		nameStart := position + 1
		if nameStart < len(raw) && raw[nameStart] == '/' {
			nameStart++
		}
		nameEnd := nameStart
		for nameEnd < len(raw) && (raw[nameEnd] >= 'a' && raw[nameEnd] <= 'z' || raw[nameEnd] >= 'A' && raw[nameEnd] <= 'Z') {
			nameEnd++
		}
		if _, blocked := disallowedTags[strings.ToLower(raw[nameStart:nameEnd])]; blocked {
			output.WriteString("&lt;")
		} else {
			output.WriteByte('<')
		}
		position++
	}
	return output.String()
}
