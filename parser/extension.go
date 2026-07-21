package parser

import (
	"context"
	"fmt"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/extension"
	"github.com/gaon12/markonward/profile"
)

type syntaxExtensionContext struct {
	state  *parseState
	offset int
	parent ast.NodeID
}

func (c *syntaxExtensionContext) Context() context.Context { return c.state.ctx }
func (c *syntaxExtensionContext) Source() []byte           { return c.state.source }
func (c *syntaxExtensionContext) Offset() int              { return c.offset }
func (c *syntaxExtensionContext) SetOffset(offset int)     { c.offset = offset }
func (c *syntaxExtensionContext) Builder() *ast.Builder    { return c.state.builder }
func (c *syntaxExtensionContext) Parent() ast.NodeID       { return c.parent }
func (c *syntaxExtensionContext) Profile() profile.Profile { return c.state.parser.profile }

func (s *parseState) parseExtensionBlock(lines []sourceLine, index int, parent ast.NodeID) (int, bool, error) {
	if len(s.parser.blockHooks) == 0 {
		return index, false, nil
	}
	content := s.lineBytes(lines[index])
	if len(content) == 0 {
		return index, false, nil
	}
	start := lineSourceOffset(lines[index], 0)
	for _, registration := range s.parser.blockHooks {
		if !extensionTriggered(registration.Triggers, content[0]) {
			continue
		}
		parseContext := &syntaxExtensionContext{state: s, offset: start, parent: parent}
		match, matched, err := registration.Handler.(extension.BlockParser).ParseBlock(parseContext)
		if err != nil {
			return index, false, fmt.Errorf("parser: block extension %s: %w", registration.ID, err)
		}
		if !matched {
			continue
		}
		consumed, err := validateExtensionMatch(registration.ID, parseContext, match, start, len(s.source)-start)
		if err != nil {
			return index, false, err
		}
		end := start + consumed
		next := index
		for next < len(lines) && end >= lines[next].end {
			if end != lines[next].end && end < lines[next].newlineEnd {
				return index, false, fmt.Errorf("parser: block extension %s stopped inside a line ending", registration.ID)
			}
			next++
			if end == lines[next-1].end || end == lines[next-1].newlineEnd {
				break
			}
		}
		if next == index || end != lines[next-1].end && end != lines[next-1].newlineEnd {
			return index, false, fmt.Errorf("parser: block extension %s must consume complete source lines", registration.ID)
		}
		if err := attachExtensionNode(s.builder, parent, match.Node, registration.ID); err != nil {
			return index, false, err
		}
		return next, true, nil
	}
	return index, false, nil
}

func (p *inlineParser) parseExtensionInline(parent ast.NodeID, position, end int) (int, bool, error) {
	if len(p.state.parser.inlineHooks) == 0 {
		return position, false, nil
	}
	for _, registration := range p.state.parser.inlineHooks {
		if !extensionTriggered(registration.Triggers, p.input.data[position]) {
			continue
		}
		start := p.input.offset(position)
		parseContext := &syntaxExtensionContext{state: p.state, offset: start, parent: parent}
		match, matched, err := registration.Handler.(extension.InlineParser).ParseInline(parseContext)
		if err != nil {
			return position, false, fmt.Errorf("parser: inline extension %s: %w", registration.ID, err)
		}
		if !matched {
			continue
		}
		consumed, err := validateExtensionMatch(registration.ID, parseContext, match, start, end-position)
		if err != nil {
			return position, false, err
		}
		if err := p.validateInlineExtensionConsumption(registration.ID, position, consumed, start, end); err != nil {
			return position, false, err
		}
		if err := attachExtensionNode(p.state.builder, parent, match.Node, registration.ID); err != nil {
			return position, false, err
		}
		return position + consumed, true, nil
	}
	return position, false, nil
}

func (p *inlineParser) validateInlineExtensionConsumption(id string, position, consumed, start, end int) error {
	if position+consumed > end || start+consumed > len(p.state.source) {
		return fmt.Errorf("parser: inline extension %s consumed beyond its inline source", id)
	}
	for offset := 0; offset < consumed; offset++ {
		logical := position + offset
		// Container parsing can join source spans after removing block markers.
		// ParseContext deliberately exposes the original source, so a syntax
		// handler may only consume bytes that remain contiguous in that source.
		if p.input.offset(logical) != start+offset || p.input.data[logical] != p.state.source[start+offset] {
			return fmt.Errorf("parser: inline extension %s crossed a non-contiguous source span", id)
		}
	}
	return nil
}

func validateExtensionMatch(id string, parseContext *syntaxExtensionContext, match extension.Match, start, available int) (int, error) {
	consumed := match.Consumed
	if consumed == 0 && parseContext.offset > start {
		consumed = parseContext.offset - start
	}
	if parseContext.offset != start && parseContext.offset != start+consumed {
		return 0, fmt.Errorf("parser: extension %s reported inconsistent offset and consumed length", id)
	}
	if consumed <= 0 || consumed > available {
		return 0, fmt.Errorf("parser: extension %s consumed %d bytes with %d available", id, consumed, available)
	}
	if match.Node == ast.NoNode {
		return 0, fmt.Errorf("parser: extension %s matched without producing a node", id)
	}
	return consumed, nil
}

func attachExtensionNode(builder *ast.Builder, parent, node ast.NodeID, id string) error {
	current := builder.Document().Node(node)
	if !current.Valid() {
		return fmt.Errorf("parser: extension %s returned invalid node %d", id, node)
	}
	if current.Parent() == ast.NoNode {
		builder.AppendChild(parent, node)
		return nil
	}
	if current.Parent() != parent {
		return fmt.Errorf("parser: extension %s returned node %d attached to a different parent", id, node)
	}
	return nil
}

func extensionTriggered(triggers []byte, current byte) bool {
	if len(triggers) == 0 {
		return true
	}
	for _, trigger := range triggers {
		if trigger == current {
			return true
		}
	}
	return false
}
