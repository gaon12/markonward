package parser

type emphasisDelimiter struct {
	start     int
	marker    byte
	original  int
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
	openers map[int]emphasisPair
}

func (p *inlineParser) planEmphasis(start, end int) emphasisPlan {
	delimiters := make([]emphasisDelimiter, 0, 8)
	p.collectEmphasisDelimiters(start, end, &delimiters)
	pairs := make(map[int]emphasisPair, len(delimiters)/2)
	for closerIndex := range delimiters {
		closer := &delimiters[closerIndex]
		if !closer.canClose {
			continue
		}
		for closer.remaining > 0 {
			openerIndex := -1
			for candidate := closerIndex - 1; candidate >= 0; candidate-- {
				opener := &delimiters[candidate]
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
			opener := &delimiters[openerIndex]
			use := 1
			if opener.remaining >= 2 && closer.remaining >= 2 {
				use = 2
			}
			openStart := opener.start + opener.remaining - use
			closeStart := closer.start + closer.closed
			pair := emphasisPair{openStart: openStart, openLength: use, closeStart: closeStart, closeLength: use, kind: closer.marker}
			pairs[openStart] = pair
			opener.remaining -= use
			closer.remaining -= use
			closer.closed += use
			// Delimiters left unmatched between a completed opener/closer pair
			// cannot later reach across the new emphasis node.
			for between := openerIndex + 1; between < closerIndex; between++ {
				delimiters[between].remaining = 0
			}
		}
	}
	return emphasisPlan{openers: pairs}
}

func emphasisOddMatch(opener, closer emphasisDelimiter) bool {
	if !closer.canOpen && !opener.canClose {
		return false
	}
	return (opener.remaining+closer.remaining)%3 == 0 && (opener.remaining%3 != 0 || closer.remaining%3 != 0)
}

func (p *inlineParser) collectEmphasisDelimiters(start, end int, delimiters *[]emphasisDelimiter) {
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
			*delimiters = append(*delimiters, emphasisDelimiter{
				start: position, marker: current, original: run, remaining: run, canOpen: canOpen, canClose: canClose,
			})
			position += run
		default:
			position++
		}
	}
}
