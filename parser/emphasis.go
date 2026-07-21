package parser

import (
	"bytes"
	"slices"
)

type emphasisDelimiter struct {
	start     int
	marker    byte
	remaining int
	closed    int
	canOpen   bool
	canClose  bool
}

type emphasisPair struct {
	openStart   int
	openLength  int
	closeStart  int
	closeLength int
	kind        byte
}

type emphasisPlan struct {
	inline   [8]emphasisPair
	count    int
	overflow []emphasisPair
}

type emphasisDelimiters struct {
	inline   [12]emphasisDelimiter
	count    int
	overflow []emphasisDelimiter
}

func (d *emphasisDelimiters) add(delimiter emphasisDelimiter) {
	if d.count < len(d.inline) {
		d.inline[d.count] = delimiter
		d.count++
		return
	}
	d.overflow = append(d.overflow, delimiter)
}

func (d *emphasisDelimiters) len() int { return d.count + len(d.overflow) }

func (d *emphasisDelimiters) at(index int) *emphasisDelimiter {
	if index < d.count {
		return &d.inline[index]
	}
	return &d.overflow[index-d.count]
}

func (p *inlineParser) planEmphasis(start, end int) emphasisPlan {
	if bytes.IndexByte(p.input.data[start:end], '*') < 0 && bytes.IndexByte(p.input.data[start:end], '_') < 0 {
		return emphasisPlan{}
	}
	var delimiters emphasisDelimiters
	p.collectEmphasisDelimiters(start, end, &delimiters)
	var plan emphasisPlan
	for closerIndex := 0; closerIndex < delimiters.len(); closerIndex++ {
		closer := delimiters.at(closerIndex)
		if !closer.canClose {
			continue
		}
		for closer.remaining > 0 {
			openerIndex := -1
			for candidate := closerIndex - 1; candidate >= 0; candidate-- {
				opener := delimiters.at(candidate)
				if opener.marker != closer.marker || !opener.canOpen || opener.remaining == 0 {
					continue
				}
				if emphasisOddMatch(*opener, *closer) {
					continue
				}
				openerIndex = candidate
				break
			}
			if openerIndex < 0 {
				break
			}
			opener := delimiters.at(openerIndex)
			use := 1
			if opener.remaining >= 2 && closer.remaining >= 2 {
				use = 2
			}
			openStart := opener.start + opener.remaining - use
			closeStart := closer.start + closer.closed
			pair := emphasisPair{openStart: openStart, openLength: use, closeStart: closeStart, closeLength: use, kind: closer.marker}
			plan.add(pair)
			opener.remaining -= use
			closer.remaining -= use
			closer.closed += use
			// Delimiters left unmatched between a completed opener/closer pair
			// cannot later reach across the new emphasis node.
			for between := openerIndex + 1; between < closerIndex; between++ {
				delimiters.at(between).remaining = 0
			}
		}
	}
	plan.prepareLookup()
	return plan
}

func (p *emphasisPlan) add(pair emphasisPair) {
	if p.count < len(p.inline) {
		p.inline[p.count] = pair
		p.count++
		return
	}
	if p.overflow == nil {
		p.overflow = make([]emphasisPair, p.count, len(p.inline)*2)
		copy(p.overflow, p.inline[:p.count])
		p.count = 0
	}
	p.overflow = append(p.overflow, pair)
}

func (p *emphasisPlan) prepareLookup() {
	if p.overflow == nil {
		return
	}
	slices.SortFunc(p.overflow, func(left, right emphasisPair) int {
		return left.openStart - right.openStart
	})
}

func (p *emphasisPlan) openerAt(position int) (emphasisPair, bool) {
	if p.overflow == nil {
		for index := 0; index < p.count; index++ {
			if p.inline[index].openStart == position {
				return p.inline[index], true
			}
		}
		return emphasisPair{}, false
	}
	low, high := 0, len(p.overflow)
	for low < high {
		middle := int(uint(low+high) >> 1)
		pair := p.overflow[middle]
		if pair.openStart < position {
			low = middle + 1
		} else {
			high = middle
		}
	}
	if low < len(p.overflow) && p.overflow[low].openStart == position {
		return p.overflow[low], true
	}
	return emphasisPair{}, false
}

func emphasisOddMatch(opener, closer emphasisDelimiter) bool {
	if !closer.canOpen && !opener.canClose {
		return false
	}
	return (opener.remaining+closer.remaining)%3 == 0 && (opener.remaining%3 != 0 || closer.remaining%3 != 0)
}

func (p *inlineParser) collectEmphasisDelimiters(start, end int, delimiters *emphasisDelimiters) {
	for position := start; position < end; {
		current := p.input.data[position]
		switch current {
		case '\\':
			position = minInt(position+2, end)
		case '`':
			if codeEnd, matched := matchingCodeSpanEnd(p.input.data, position, end); matched {
				position = codeEnd
			} else {
				position += byteRun(p.input.data, position, end, '`')
			}
		case '<':
			if angleEnd, matched := matchingAngleSpanEnd(p.input.data, position, end); matched {
				position = angleEnd
			} else {
				position++
			}
		case '!', '[':
			labelOpen := position
			if current == '!' {
				if position+1 >= end || p.input.data[position+1] != '[' {
					position++
					continue
				}
				labelOpen++
			}
			labelClose := findClosingBracket(p.input.data, labelOpen+1, end)
			if labelClose < 0 {
				position++
				continue
			}
			targetEnd, matched := p.linkSyntaxEnd(labelOpen, end)
			if !matched {
				position++
				continue
			}
			rejectedByNestedLink := current == '[' && p.containsNestedLink(labelOpen+1, labelClose)
			if rejectedByNestedLink {
				p.collectEmphasisDelimiters(labelOpen+1, labelClose, delimiters)
				position = labelClose + 1
			} else {
				position = targetEnd
			}
		case '*', '_':
			run := byteRun(p.input.data, position, end, current)
			canOpen, canClose := delimiterFlanking(p.input.data, position, run, current, 0, end)
			delimiters.add(emphasisDelimiter{
				start: position, marker: current, remaining: run, canOpen: canOpen, canClose: canClose,
			})
			position += run
		default:
			position++
		}
	}
}
