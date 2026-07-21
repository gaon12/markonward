// Package markdown renders AST documents as normalized Markdown.
package markdown

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/profile"
)

// Renderer emits deterministic Markdown for one target profile.
type Renderer struct{ profile profile.Profile }

// New constructs a renderer. An invalid profile is reported by Render.
func New(selected profile.Profile) *Renderer { return &Renderer{profile: selected} }

// Render writes normalized Markdown.
func (r *Renderer) Render(ctx context.Context, writer io.Writer, document *ast.Document) error {
	if r == nil || !r.profile.Valid() {
		return fmt.Errorf("markdown: a target profile is required")
	}
	if ctx == nil || writer == nil || document == nil {
		return fmt.Errorf("markdown: context, writer, and document are required")
	}
	state := renderState{renderer: r, ctx: ctx, document: document}
	if err := state.blocks(document.Root()); err != nil {
		return err
	}
	result := strings.TrimRight(state.output.String(), " \t\n")
	if result != "" {
		result += "\n"
	}
	_, err := io.WriteString(writer, result)
	return err
}

type renderState struct {
	renderer *Renderer
	ctx      context.Context
	document *ast.Document
	output   strings.Builder
}

func (s *renderState) blocks(parent ast.NodeID) error {
	for child := s.document.Node(parent).FirstChild(); child != ast.NoNode; child = s.document.Node(child).NextSibling() {
		if err := s.block(child); err != nil {
			return err
		}
	}
	return nil
}

func (s *renderState) block(id ast.NodeID) error { //nolint:gocyclo // The switch is the normalized block grammar table.
	if err := s.ctx.Err(); err != nil {
		return err
	}
	node := s.document.Node(id)
	switch node.Kind() {
	case ast.Paragraph:
		if err := s.inlines(id); err != nil {
			return err
		}
		s.blankLine()
	case ast.Heading:
		level, _ := node.Integers()
		if level < 1 || level > 6 {
			return fmt.Errorf("markdown: invalid heading level %d", level)
		}
		s.output.WriteString(strings.Repeat("#", level) + " ")
		if err := s.inlines(id); err != nil {
			return err
		}
		s.blankLine()
	case ast.BlockQuote:
		content, err := s.renderBlocksToString(id)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
			s.output.WriteString(">")
			if line != "" {
				s.output.WriteByte(' ')
				s.output.WriteString(line)
			}
			s.output.WriteByte('\n')
		}
		s.blankLine()
	case ast.List:
		if err := s.renderList(id, 0); err != nil {
			return err
		}
		s.blankLine()
	case ast.ListItem:
		return fmt.Errorf("markdown: list item outside list")
	case ast.ThematicBreak:
		s.output.WriteString("---\n\n")
	case ast.CodeBlock:
		fence := strings.Repeat("`", maxInt(3, longestRun(node.Text(), '`')+1))
		s.output.WriteString(fence)
		if info := strings.TrimSpace(node.Title()); info != "" {
			s.output.WriteString(info)
		}
		s.output.WriteByte('\n')
		s.output.WriteString(node.Text())
		if !strings.HasSuffix(node.Text(), "\n") {
			s.output.WriteByte('\n')
		}
		s.output.WriteString(fence + "\n\n")
	case ast.HTMLBlock:
		s.output.WriteString(node.Text())
		s.blankLine()
	case ast.Table:
		if !s.renderer.profile.Has(profile.Tables) {
			return fmt.Errorf("markdown: target profile %s does not support tables", s.renderer.profile.ID())
		}
		if err := s.renderTable(id); err != nil {
			return err
		}
		s.blankLine()
	case ast.Invalid, ast.DocumentKind, ast.Text, ast.SoftBreak, ast.HardBreak, ast.CodeSpan, ast.Emphasis, ast.Strong, ast.Strikethrough, ast.Link, ast.Image, ast.Autolink, ast.RawHTML, ast.TableHead, ast.TableBody, ast.TableRow, ast.TableCell, ast.TaskCheck:
		return fmt.Errorf("markdown: unexpected block node %s", node.Kind())
	case ast.Custom:
		return fmt.Errorf("markdown: no handler for custom node %q", node.CustomKind())
	default:
		return fmt.Errorf("markdown: unsupported node %s", node.Kind())
	}
	return nil
}

func (s *renderState) inlines(parent ast.NodeID) error {
	for child := s.document.Node(parent).FirstChild(); child != ast.NoNode; child = s.document.Node(child).NextSibling() {
		if err := s.inline(child); err != nil {
			return err
		}
	}
	return nil
}

func (s *renderState) inline(id ast.NodeID) error { //nolint:gocyclo // The switch is the normalized inline grammar table.
	if err := s.ctx.Err(); err != nil {
		return err
	}
	node := s.document.Node(id)
	switch node.Kind() {
	case ast.Text:
		s.output.WriteString(escapeText(node.Text()))
	case ast.SoftBreak:
		s.output.WriteByte('\n')
	case ast.HardBreak:
		s.output.WriteString("\\\n")
	case ast.CodeSpan:
		delimiter := strings.Repeat("`", longestRun(node.Text(), '`')+1)
		content := node.Text()
		if strings.HasPrefix(content, "`") || strings.HasSuffix(content, "`") || strings.HasPrefix(content, " ") && strings.HasSuffix(content, " ") {
			content = " " + content + " "
		}
		s.output.WriteString(delimiter + content + delimiter)
	case ast.Emphasis:
		return s.inlineContainer(id, "*", "*")
	case ast.Strong:
		return s.inlineContainer(id, "**", "**")
	case ast.Strikethrough:
		if !s.renderer.profile.Has(profile.Strikethrough) {
			return fmt.Errorf("markdown: target profile %s does not support strikethrough", s.renderer.profile.ID())
		}
		return s.inlineContainer(id, "~~", "~~")
	case ast.Link:
		s.output.WriteByte('[')
		if err := s.inlines(id); err != nil {
			return err
		}
		s.output.WriteString("](" + escapeDestination(node.Destination()))
		if node.Title() != "" {
			s.output.WriteString(` "` + strings.ReplaceAll(node.Title(), `"`, `\"`) + `"`)
		}
		s.output.WriteByte(')')
	case ast.Image:
		s.output.WriteString("![")
		if err := s.inlines(id); err != nil {
			return err
		}
		s.output.WriteString("](" + escapeDestination(node.Destination()))
		if node.Title() != "" {
			s.output.WriteString(` "` + strings.ReplaceAll(node.Title(), `"`, `\"`) + `"`)
		}
		s.output.WriteByte(')')
	case ast.Autolink:
		s.output.WriteByte('<')
		s.output.WriteString(strings.TrimPrefix(node.Destination(), "mailto:"))
		s.output.WriteByte('>')
	case ast.RawHTML:
		s.output.WriteString(node.Text())
	case ast.TaskCheck:
		if node.Flags()&ast.TaskChecked != 0 {
			s.output.WriteString("[x] ")
		} else {
			s.output.WriteString("[ ] ")
		}
	case ast.TableCell:
		return s.inlines(id)
	case ast.Invalid, ast.DocumentKind, ast.Paragraph, ast.Heading, ast.BlockQuote, ast.List, ast.ListItem, ast.ThematicBreak, ast.CodeBlock, ast.HTMLBlock, ast.Table, ast.TableHead, ast.TableBody, ast.TableRow:
		return fmt.Errorf("markdown: unexpected inline node %s", node.Kind())
	case ast.Custom:
		return fmt.Errorf("markdown: no handler for custom node %q", node.CustomKind())
	default:
		return fmt.Errorf("markdown: unsupported inline node %s", node.Kind())
	}
	return nil
}

func (s *renderState) inlineContainer(id ast.NodeID, open, close string) error {
	s.output.WriteString(open)
	if err := s.inlines(id); err != nil {
		return err
	}
	s.output.WriteString(close)
	return nil
}

func (s *renderState) renderList(list ast.NodeID, depth int) error {
	node := s.document.Node(list)
	ordered := node.Flags()&ast.ListOrdered != 0
	start, _ := node.Integers()
	if start == 0 {
		start = 1
	}
	index := 0
	for item := node.FirstChild(); item != ast.NoNode; item = s.document.Node(item).NextSibling() {
		prefix := "- "
		if ordered {
			prefix = strconv.Itoa(start+index) + ". "
		}
		content, err := s.renderItemToString(item)
		if err != nil {
			return err
		}
		lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
		for lineIndex, line := range lines {
			s.output.WriteString(strings.Repeat("  ", depth))
			if lineIndex == 0 {
				s.output.WriteString(prefix)
			} else {
				s.output.WriteString(strings.Repeat(" ", len(prefix)))
			}
			s.output.WriteString(line)
			s.output.WriteByte('\n')
		}
		index++
	}
	return nil
}

func (s *renderState) renderItemToString(item ast.NodeID) (string, error) {
	nested := renderState{renderer: s.renderer, ctx: s.ctx, document: s.document}
	for child := s.document.Node(item).FirstChild(); child != ast.NoNode; child = s.document.Node(child).NextSibling() {
		kind := s.document.Node(child).Kind()
		if kind == ast.TaskCheck {
			if err := nested.inline(child); err != nil {
				return "", err
			}
			continue
		}
		if err := nested.block(child); err != nil {
			return "", err
		}
	}
	return strings.TrimRight(nested.output.String(), "\n"), nil
}

func (s *renderState) renderTable(table ast.NodeID) error {
	head := s.document.Node(table).FirstChild()
	if head == ast.NoNode || s.document.Node(head).Kind() != ast.TableHead {
		return fmt.Errorf("markdown: table has no header")
	}
	headerRow := s.document.Node(head).FirstChild()
	if headerRow == ast.NoNode {
		return fmt.Errorf("markdown: table header is empty")
	}
	if err := s.renderTableRow(headerRow); err != nil {
		return err
	}
	s.output.WriteByte('|')
	for cell := s.document.Node(headerRow).FirstChild(); cell != ast.NoNode; cell = s.document.Node(cell).NextSibling() {
		s.output.WriteString(" " + tableDelimiter(s.document.Node(cell).Flags()) + " |")
	}
	s.output.WriteByte('\n')
	body := s.document.Node(head).NextSibling()
	if body != ast.NoNode {
		for row := s.document.Node(body).FirstChild(); row != ast.NoNode; row = s.document.Node(row).NextSibling() {
			if err := s.renderTableRow(row); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *renderState) renderTableRow(row ast.NodeID) error {
	s.output.WriteByte('|')
	for cell := s.document.Node(row).FirstChild(); cell != ast.NoNode; cell = s.document.Node(cell).NextSibling() {
		nested := renderState{renderer: s.renderer, ctx: s.ctx, document: s.document}
		if err := nested.inlines(cell); err != nil {
			return err
		}
		content := strings.ReplaceAll(nested.output.String(), "|", "\\|")
		s.output.WriteString(" " + content + " |")
	}
	s.output.WriteByte('\n')
	return nil
}

func tableDelimiter(flags uint32) string {
	switch {
	case flags&ast.TableAlignCenter != 0:
		return ":---:"
	case flags&ast.TableAlignLeft != 0:
		return ":---"
	case flags&ast.TableAlignRight != 0:
		return "---:"
	default:
		return "---"
	}
}

func (s *renderState) renderBlocksToString(parent ast.NodeID) (string, error) {
	nested := renderState{renderer: s.renderer, ctx: s.ctx, document: s.document}
	if err := nested.blocks(parent); err != nil {
		return "", err
	}
	return nested.output.String(), nil
}

func (s *renderState) blankLine() {
	trimmed := strings.TrimRight(s.output.String(), " \t\n")
	s.output.Reset()
	s.output.WriteString(trimmed)
	if trimmed != "" {
		s.output.WriteString("\n\n")
	}
}

func escapeText(value string) string {
	var output strings.Builder
	output.Grow(len(value))
	for _, current := range value {
		if strings.ContainsRune(`\`+"`*_{}[]<>#+-.!|~", current) {
			output.WriteByte('\\')
		}
		output.WriteRune(current)
	}
	return output.String()
}

func escapeDestination(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, ")", "\\)")
	return value
}

func longestRun(value string, marker byte) int {
	longest, current := 0, 0
	for index := 0; index < len(value); index++ {
		if value[index] == marker {
			current++
			longest = maxInt(longest, current)
		} else {
			current = 0
		}
	}
	return longest
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
