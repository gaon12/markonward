package ast

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"unicode/utf8"
)

// NodeID is stable for the lifetime of one Document. Zero is not a node.
type NodeID uint32

const NoNode NodeID = 0

type nodeRecord struct {
	kind                                Kind
	span                                sourceSpan
	content                             sourceSpan
	parent, first, last, previous, next NodeID
	flags                               uint32
	integer1, integer2                  int32
}

type sourceSpan struct {
	start uint32
	end   uint32
}

func packSpan(span Span) sourceSpan {
	return sourceSpan{start: uint32(span.Start), end: uint32(span.End)} // #nosec G115 -- Builder caps sources at MaxUint32.
}

func (s sourceSpan) public() Span { return Span{Start: int(s.start), End: int(s.end)} }

type nodePayload struct {
	text        string
	destination string
	title       string
	customKind  string
	custom      any
	hasText     bool
}

// Document owns an arena of Markdown nodes and a reference to their source.
// Documents returned by Parse borrow the input byte slice; callers must not
// mutate it while the document is in use.
type Document struct {
	profile  string
	source   []byte
	borrowed bool
	nodes    []nodeRecord
	payloads map[NodeID]nodePayload

	lineOnce   sync.Once
	lineStarts []int
}

// Profile returns the stable profile identifier used to create d.
func (d *Document) Profile() string { return d.profile }

// Source returns the source referenced by d. The returned bytes are read-only.
func (d *Document) Source() []byte { return d.source }

// BorrowsSource reports whether d references caller-owned source bytes.
func (d *Document) BorrowsSource() bool { return d.borrowed }

// Root returns the document root node.
func (d *Document) Root() NodeID {
	if len(d.nodes) <= 1 {
		return NoNode
	}
	return 1
}

// Len returns the number of nodes, excluding the invalid zero slot.
func (d *Document) Len() int { return len(d.nodes) - 1 }

// Node returns a read-only view of id.
func (d *Document) Node(id NodeID) Node {
	if !d.valid(id) {
		return Node{}
	}
	return Node{document: d, id: id}
}

func (d *Document) valid(id NodeID) bool { return id > 0 && int(id) < len(d.nodes) }

// Position converts a byte offset into a one-based line and rune column.
func (d *Document) Position(offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(d.source) {
		offset = len(d.source)
	}
	d.lineOnce.Do(d.indexLines)
	lineIndex := sort.Search(len(d.lineStarts), func(i int) bool {
		return d.lineStarts[i] > offset
	}) - 1
	if lineIndex < 0 {
		lineIndex = 0
	}
	start := d.lineStarts[lineIndex]
	return Position{
		Offset: offset,
		Line:   lineIndex + 1,
		Column: utf8.RuneCount(d.source[start:offset]) + 1,
	}
}

func (d *Document) indexLines() {
	d.lineStarts = []int{0}
	for i, b := range d.source {
		if b == '\n' {
			d.lineStarts = append(d.lineStarts, i+1)
		}
	}
}

// Walk traverses the tree in document order. The callback is invoked on entry
// and exit for container nodes. Returning a non-nil error stops traversal.
func (d *Document) Walk(root NodeID, fn func(Node, bool) error) error {
	if !d.valid(root) {
		return fmt.Errorf("ast: invalid walk root %d", root)
	}
	type frame struct {
		id      NodeID
		exiting bool
	}
	stack := []frame{{id: root}}
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if current.exiting {
			if err := fn(d.Node(current.id), false); err != nil {
				return err
			}
			continue
		}
		if err := fn(d.Node(current.id), true); err != nil {
			return err
		}
		stack = append(stack, frame{id: current.id, exiting: true})
		for child := d.nodes[current.id].last; child != NoNode; child = d.nodes[child].previous {
			stack = append(stack, frame{id: child})
		}
	}
	return nil
}

// Validate checks arena and tree invariants.
func (d *Document) Validate() error {
	if d.Root() == NoNode || d.nodes[d.Root()].kind != DocumentKind {
		return errors.New("ast: missing document root")
	}
	for id := NodeID(1); int(id) < len(d.nodes); id++ {
		record := d.nodes[id]
		span := record.span.public()
		if span.End < span.Start || span.End > len(d.source) {
			return fmt.Errorf("ast: node %d has invalid span [%d,%d)", id, span.Start, span.End)
		}
		if record.parent != NoNode && !d.valid(record.parent) {
			return fmt.Errorf("ast: node %d has invalid parent %d", id, record.parent)
		}
		if record.next != NoNode && d.nodes[record.next].previous != id {
			return fmt.Errorf("ast: node %d sibling linkage is inconsistent", id)
		}
	}
	return nil
}

// Node is a small read-only handle into a Document arena.
type Node struct {
	document *Document
	id       NodeID
}

func (n Node) Valid() bool { return n.document != nil && n.document.valid(n.id) }
func (n Node) ID() NodeID  { return n.id }
func (n Node) Kind() Kind {
	if !n.Valid() {
		return Invalid
	}
	return n.document.nodes[n.id].kind
}
func (n Node) Span() Span {
	if !n.Valid() {
		return Span{}
	}
	return n.document.nodes[n.id].span.public()
}
func (n Node) ContentSpan() Span {
	if !n.Valid() {
		return Span{}
	}
	return n.document.nodes[n.id].content.public()
}
func (n Node) Parent() NodeID {
	if !n.Valid() {
		return NoNode
	}
	return n.document.nodes[n.id].parent
}
func (n Node) FirstChild() NodeID {
	if !n.Valid() {
		return NoNode
	}
	return n.document.nodes[n.id].first
}
func (n Node) LastChild() NodeID {
	if !n.Valid() {
		return NoNode
	}
	return n.document.nodes[n.id].last
}
func (n Node) PreviousSibling() NodeID {
	if !n.Valid() {
		return NoNode
	}
	return n.document.nodes[n.id].previous
}
func (n Node) NextSibling() NodeID {
	if !n.Valid() {
		return NoNode
	}
	return n.document.nodes[n.id].next
}
func (n Node) Flags() uint32 {
	if !n.Valid() {
		return 0
	}
	return n.document.nodes[n.id].flags
}
func (n Node) Text() string {
	if !n.Valid() {
		return ""
	}
	payload := n.document.payloads[n.id]
	if payload.hasText {
		return payload.text
	}
	span := n.document.nodes[n.id].content.public()
	if span.End <= len(n.document.source) && span.Start <= span.End {
		return string(n.document.source[span.Start:span.End])
	}
	return ""
}
func (n Node) Destination() string {
	if !n.Valid() {
		return ""
	}
	return n.document.payloads[n.id].destination
}
func (n Node) Title() string {
	if !n.Valid() {
		return ""
	}
	return n.document.payloads[n.id].title
}
func (n Node) Integers() (int, int) {
	if !n.Valid() {
		return 0, 0
	}
	record := n.document.nodes[n.id]
	return int(record.integer1), int(record.integer2)
}
func (n Node) CustomKind() string {
	if !n.Valid() {
		return ""
	}
	return n.document.payloads[n.id].customKind
}
func (n Node) CustomPayload() any {
	if !n.Valid() {
		return nil
	}
	return n.document.payloads[n.id].custom
}
