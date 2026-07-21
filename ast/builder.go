package ast

import (
	"fmt"
	"math"
)

// Builder constructs a Document while preserving arena invariants.
type Builder struct {
	document *Document
	built    bool
}

// NewBuilder creates a builder. If borrow is true, the finished document
// references source directly; otherwise source is copied immediately.
func NewBuilder(profile string, source []byte, borrow bool) *Builder {
	if uint64(len(source)) > math.MaxUint32 {
		panic("ast: source exceeds uint32 arena offsets")
	}
	if !borrow {
		source = append([]byte(nil), source...)
	}
	document := &Document{
		profile:  profile,
		source:   source,
		borrowed: borrow,
		// Twenty-four nodes cover typical short documents. Larger documents grow
		// geometrically without making every parser-only call retain unused arena
		// records.
		nodes: make([]nodeRecord, 1, 24),
	}
	builder := &Builder{document: document}
	root := builder.Add(DocumentKind, Span{Start: 0, End: len(source)})
	if root != 1 {
		panic("ast: document root did not receive node ID 1")
	}
	return builder
}

// Document returns the document under construction.
func (b *Builder) Document() *Document { return b.document }

// Add allocates a node in the arena.
func (b *Builder) Add(kind Kind, span Span) NodeID {
	b.assertMutable()
	if kind == Invalid {
		panic("ast: cannot add an invalid node")
	}
	if span.Start < 0 || span.End < span.Start || span.End > len(b.document.source) {
		panic(fmt.Sprintf("ast: invalid node span [%d,%d)", span.Start, span.End))
	}
	if len(b.document.nodes) > math.MaxUint32 {
		panic("ast: document contains too many nodes")
	}
	// #nosec G115 -- the explicit MaxUint32 guard makes the conversion safe.
	id := NodeID(len(b.document.nodes))
	packed := packSpan(span)
	b.document.nodes = append(b.document.nodes, nodeRecord{kind: kind, span: packed, content: packed})
	return id
}

// AppendChild appends child to parent, detaching child from any prior parent.
func (b *Builder) AppendChild(parent, child NodeID) {
	b.assertMutable()
	b.require(parent)
	b.require(child)
	if parent == child {
		panic("ast: a node cannot parent itself")
	}
	if b.document.nodes[child].parent != NoNode {
		b.Detach(child)
	}
	p := &b.document.nodes[parent]
	c := &b.document.nodes[child]
	c.parent = parent
	c.previous = p.last
	if p.last != NoNode {
		b.document.nodes[p.last].next = child
	} else {
		p.first = child
	}
	p.last = child
}

// Detach removes id from its parent and sibling list.
func (b *Builder) Detach(id NodeID) {
	b.assertMutable()
	b.require(id)
	record := &b.document.nodes[id]
	if record.parent == NoNode {
		return
	}
	parent := &b.document.nodes[record.parent]
	if record.previous != NoNode {
		b.document.nodes[record.previous].next = record.next
	} else {
		parent.first = record.next
	}
	if record.next != NoNode {
		b.document.nodes[record.next].previous = record.previous
	} else {
		parent.last = record.previous
	}
	record.parent, record.previous, record.next = NoNode, NoNode, NoNode
}

func (b *Builder) SetContentSpan(id NodeID, span Span) {
	b.assertMutable()
	b.require(id)
	if span.Start < 0 || span.End < span.Start || span.End > len(b.document.source) {
		panic("ast: invalid content span")
	}
	b.document.nodes[id].content = packSpan(span)
}

func (b *Builder) SetText(id NodeID, text string) {
	b.assertMutable()
	b.require(id)
	payload := b.payload(id)
	payload.text = text
	payload.hasText = true
	b.document.payloads[id] = payload
}

func (b *Builder) SetDestination(id NodeID, destination string) {
	b.assertMutable()
	b.require(id)
	payload := b.payload(id)
	payload.destination = destination
	b.document.payloads[id] = payload
}

func (b *Builder) SetTitle(id NodeID, title string) {
	b.assertMutable()
	b.require(id)
	payload := b.payload(id)
	payload.title = title
	b.document.payloads[id] = payload
}

func (b *Builder) SetIntegers(id NodeID, first, second int) {
	b.assertMutable()
	b.require(id)
	const (
		minimum = -1 << 31
		maximum = 1<<31 - 1
	)
	if first < minimum || first > maximum || second < minimum || second > maximum {
		panic("ast: integer metadata exceeds int32 storage")
	}
	b.document.nodes[id].integer1 = int32(first)  // #nosec G115 -- parser metadata is range-validated before assignment.
	b.document.nodes[id].integer2 = int32(second) // #nosec G115 -- parser metadata is range-validated before assignment.
}

func (b *Builder) SetFlags(id NodeID, flags uint32) {
	b.assertMutable()
	b.require(id)
	b.document.nodes[id].flags = flags
}

func (b *Builder) SetCustom(id NodeID, kind string, payload any) {
	b.assertMutable()
	b.require(id)
	if b.document.nodes[id].kind != Custom {
		panic("ast: custom payload requires a custom node")
	}
	stored := b.payload(id)
	stored.customKind = kind
	stored.custom = payload
	b.document.payloads[id] = stored
}

// Build validates and freezes the document.
func (b *Builder) Build() (*Document, error) {
	b.assertMutable()
	if err := b.document.Validate(); err != nil {
		return nil, err
	}
	b.built = true
	return b.document, nil
}

func (b *Builder) assertMutable() {
	if b == nil || b.document == nil || b.built {
		panic("ast: builder is nil or already built")
	}
}

func (b *Builder) require(id NodeID) {
	if !b.document.valid(id) {
		panic(fmt.Sprintf("ast: invalid node ID %d", id))
	}
}

func (b *Builder) payload(id NodeID) nodePayload {
	if b.document.payloads == nil {
		b.document.payloads = make(map[NodeID]nodePayload)
	}
	return b.document.payloads[id]
}
