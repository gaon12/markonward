// Package ast defines Markonward's source-aware Markdown document tree.
package ast

import "fmt"

// Kind identifies a built-in Markdown node type.
type Kind uint16

const (
	Invalid Kind = iota
	DocumentKind
	Paragraph
	Heading
	BlockQuote
	List
	ListItem
	ThematicBreak
	CodeBlock
	HTMLBlock
	Text
	SoftBreak
	HardBreak
	CodeSpan
	Emphasis
	Strong
	Strikethrough
	Link
	Image
	Autolink
	RawHTML
	Table
	TableHead
	TableBody
	TableRow
	TableCell
	TaskCheck
	Custom
)

// Built-in node flag bits. Flags are meaningful only for the documented kind.
const (
	ListOrdered uint32 = 1 << iota
	ListTight
	TaskChecked
	TableAlignLeft
	TableAlignCenter
	TableAlignRight
	// InlineDelimiterUnderscore records that an Emphasis or Strong node uses
	// the underscore delimiter. An unset bit selects the asterisk delimiter.
	InlineDelimiterUnderscore
	// StrikethroughSingleDelimiter records EnhanceMark's single-tilde form.
	// An unset bit selects the GFM double-tilde delimiter.
	StrikethroughSingleDelimiter
	// InlineRecoveredDelimiter marks an inline container synthesized by
	// paragraph-end recovery rather than matched source delimiters.
	InlineRecoveredDelimiter
)

var kindNames = [...]string{
	Invalid:       "invalid",
	DocumentKind:  "document",
	Paragraph:     "paragraph",
	Heading:       "heading",
	BlockQuote:    "block_quote",
	List:          "list",
	ListItem:      "list_item",
	ThematicBreak: "thematic_break",
	CodeBlock:     "code_block",
	HTMLBlock:     "html_block",
	Text:          "text",
	SoftBreak:     "soft_break",
	HardBreak:     "hard_break",
	CodeSpan:      "code_span",
	Emphasis:      "emphasis",
	Strong:        "strong",
	Strikethrough: "strikethrough",
	Link:          "link",
	Image:         "image",
	Autolink:      "autolink",
	RawHTML:       "raw_html",
	Table:         "table",
	TableHead:     "table_head",
	TableBody:     "table_body",
	TableRow:      "table_row",
	TableCell:     "table_cell",
	TaskCheck:     "task_check",
	Custom:        "custom",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) && kindNames[k] != "" {
		return kindNames[k]
	}
	return fmt.Sprintf("kind(%d)", k)
}

// IsBlock reports whether k is a built-in block node.
func (k Kind) IsBlock() bool {
	return k >= DocumentKind && k <= HTMLBlock || k >= Table && k <= TableRow
}

// IsInline reports whether k is a built-in inline node.
func (k Kind) IsInline() bool {
	return k >= Text && k <= RawHTML || k == TableCell || k == TaskCheck
}
