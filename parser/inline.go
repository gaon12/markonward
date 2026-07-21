package parser

import (
	"bytes"
	"fmt"
	"html"
	"net/mail"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/diagnostic"
	"github.com/gaon12/markonward/profile"
	"github.com/gaon12/markonward/trace"
)

type inlineInput struct {
	data     []byte
	offsets  []int
	terminal int
}

func newInlineInput(source []byte, spans []ast.Span) inlineInput {
	if len(spans) == 1 {
		span := spans[0]
		return inlineInput{data: source[span.Start:span.End], terminal: span.End}
	}
	if spansAreLFContiguous(source, spans) {
		start := spans[0].Start
		end := spans[len(spans)-1].End
		return inlineInput{data: source[start:end], terminal: end}
	}
	total := 0
	for _, span := range spans {
		total += span.Len()
	}
	total += len(spans) - 1
	data := make([]byte, 0, total)
	offsets := make([]int, 0, total)
	for spanIndex, span := range spans {
		if spanIndex > 0 {
			data = append(data, '\n')
			offsets = append(offsets, spans[spanIndex-1].End)
		}
		data = append(data, source[span.Start:span.End]...)
		for offset := span.Start; offset < span.End; offset++ {
			offsets = append(offsets, offset)
		}
	}
	terminal := 0
	if len(spans) > 0 {
		terminal = spans[len(spans)-1].End
	}
	return inlineInput{data: data, offsets: offsets, terminal: terminal}
}

func spansAreLFContiguous(source []byte, spans []ast.Span) bool {
	if len(spans) < 2 {
		return false
	}
	for index := 1; index < len(spans); index++ {
		previous := spans[index-1]
		if previous.End >= len(source) || source[previous.End] != '\n' || spans[index].Start != previous.End+1 {
			return false
		}
	}
	return true
}

func (i inlineInput) offset(index int) int {
	if index <= 0 {
		if len(i.offsets) > 0 {
			return i.offsets[0]
		}
		return i.terminal - len(i.data)
	}
	if index >= len(i.data) {
		return i.terminal
	}
	if len(i.offsets) == 0 {
		return i.terminal - len(i.data) + index
	}
	return i.offsets[index]
}

func (i inlineInput) span(start, end int) ast.Span {
	return ast.Span{Start: i.offset(start), End: i.offset(end)}
}

func (s *parseState) parseInlines() error {
	for _, block := range s.inlines {
		if err := s.checkContext(); err != nil {
			return err
		}
		input := newInlineInput(s.source, block.spans)
		inline := inlineParser{state: s, input: input}
		if err := inline.parseRange(block.node, 0, len(input.data)); err != nil {
			return err
		}
	}
	return nil
}

type inlineParser struct {
	state *parseState
	input inlineInput
}

func (p *inlineParser) parseRange(parent ast.NodeID, start, end int) error {
	for position := start; position < end; {
		if err := p.state.checkContext(); err != nil {
			return err
		}
		current := p.input.data[position]
		switch {
		case current == '\n':
			p.appendBreak(parent, position, false)
			position++
		case current == '\\' && position+1 < end && isASCIIPunctuation(p.input.data[position+1]):
			p.appendLiteral(parent, position, position+2, string(p.input.data[position+1]))
			position += 2
		case current == '\\' && position+1 < end && p.input.data[position+1] == '\n':
			p.appendBreak(parent, position, true)
			position += 2
		case current == '`':
			next, matched, err := p.parseCodeSpan(parent, position, end)
			if err != nil {
				return err
			}
			if matched {
				position = next
			} else {
				p.appendSource(parent, position, position+1)
				position++
			}
		case current == '!' && position+1 < end && p.input.data[position+1] == '[':
			next, matched, err := p.parseLink(parent, position, end, true)
			if err != nil {
				return err
			}
			if matched {
				position = next
			} else {
				p.appendSource(parent, position, position+1)
				position++
			}
		case current == '[':
			next, matched, err := p.parseLink(parent, position, end, false)
			if err != nil {
				return err
			}
			if matched {
				position = next
			} else {
				p.appendSource(parent, position, position+1)
				position++
			}
		case current == '<':
			next, matched := p.parseAngle(parent, position, end)
			if matched {
				position = next
			} else {
				p.appendSource(parent, position, position+1)
				position++
			}
		case current == '&':
			next, matched := p.parseEntity(parent, position, end)
			if matched {
				position = next
			} else {
				p.appendSource(parent, position, position+1)
				position++
			}
		case current == '*' || current == '_':
			next, matched, err := p.parseDelimiter(parent, position, end, current)
			if err != nil {
				return err
			}
			if matched {
				position = next
			} else {
				run := byteRun(p.input.data, position, end, current)
				p.appendSource(parent, position, position+run)
				position += run
			}
		case current == '~' && p.state.parser.profile.Has(profile.Strikethrough):
			next, matched, err := p.parseDelimiter(parent, position, end, current)
			if err != nil {
				return err
			}
			if matched {
				position = next
			} else {
				run := byteRun(p.input.data, position, end, current)
				p.appendSource(parent, position, position+run)
				position += run
			}
		case p.state.parser.profile.Has(profile.ExtendedAutolinks):
			next, matched := p.parseExtendedAutolink(parent, position, end)
			if matched {
				position = next
			} else {
				position = p.appendPlainRun(parent, position, end)
			}
		default:
			position = p.appendPlainRun(parent, position, end)
		}
	}
	return nil
}

func (p *inlineParser) appendPlainRun(parent ast.NodeID, start, end int) int {
	position := start + 1
	for position < end && !isInlineSpecial(p.input.data[position], p.state.parser.profile) {
		if p.state.parser.profile.Has(profile.ExtendedAutolinks) && isAutolinkBoundary(p.input.data, position) && hasAutolinkPrefix(p.input.data[position:end]) {
			break
		}
		position++
	}
	if position < end && p.input.data[position] == '\n' {
		textEnd := position
		spaces := 0
		for textEnd-spaces-1 >= start && p.input.data[textEnd-spaces-1] == ' ' {
			spaces++
		}
		if spaces >= 2 {
			p.appendSource(parent, start, textEnd-spaces)
			p.appendBreak(parent, position, true)
			return position + 1
		}
	}
	p.appendSource(parent, start, position)
	return position
}

func isAutolinkBoundary(data []byte, position int) bool {
	if position == 0 {
		return true
	}
	previous, _ := previousRune(data, position, 0)
	return unicode.IsSpace(previous) || isUnicodePunctuation(previous)
}

func hasAutolinkPrefix(data []byte) bool {
	return bytes.HasPrefix(data, []byte("http://")) || bytes.HasPrefix(data, []byte("https://")) || bytes.HasPrefix(data, []byte("www."))
}

func isInlineSpecial(current byte, selected profile.Profile) bool {
	if current == '\n' || current == '\\' || current == '`' || current == '!' || current == '[' || current == '<' || current == '&' || current == '*' || current == '_' {
		return true
	}
	return current == '~' && selected.Has(profile.Strikethrough)
}

func (p *inlineParser) parseCodeSpan(parent ast.NodeID, start, end int) (int, bool, error) {
	run := byteRun(p.input.data, start, end, '`')
	for candidate := start + run; candidate < end; candidate++ {
		if p.input.data[candidate] != '`' {
			continue
		}
		closing := byteRun(p.input.data, candidate, end, '`')
		if closing != run {
			candidate += closing - 1
			continue
		}
		literal := strings.ReplaceAll(string(p.input.data[start+run:candidate]), "\n", " ")
		if len(literal) >= 2 && literal[0] == ' ' && literal[len(literal)-1] == ' ' && strings.Trim(literal, " ") != "" {
			literal = literal[1 : len(literal)-1]
		}
		node := p.state.builder.Add(ast.CodeSpan, p.input.span(start, candidate+run))
		p.state.builder.SetText(node, literal)
		p.state.builder.AppendChild(parent, node)
		return candidate + run, true, p.emitNode(node, ast.CodeSpan)
	}
	return p.recoverUnclosed(parent, start, end, run, ast.CodeSpan)
}

func (p *inlineParser) parseDelimiter(parent ast.NodeID, start, end int, marker byte) (int, bool, error) {
	run := byteRun(p.input.data, start, end, marker)
	if marker == '~' && run > 2 {
		return start, false, nil
	}
	if marker == '~' && run == 1 && p.state.parser.profile.Has(profile.KoreanRangeInference) && p.isRangeSeparator(start, end) {
		if err := p.state.emit(trace.Decisions, trace.Event{
			Phase: trace.Inline, RuleID: "enhance.inline.tilde.range", Decision: trace.Literal, Span: p.input.span(start, start+1),
		}); err != nil {
			return 0, false, err
		}
		return start, false, nil
	}
	canOpen, _ := delimiterFlanking(p.input.data, start, run, marker, 0, end)
	if err := p.emitDelimiterFound(start, run, marker); err != nil {
		return 0, false, err
	}
	match := p.findClosing(start, end, run, marker)
	enhancedPair := false
	if !canOpen && match.found && marker != '~' && p.state.parser.profile.Has(profile.PairedPunctuationEmphasis) {
		enhancedPair = pairedPunctuation(p.input.data[start+match.openLength : match.contentEnd])
	}
	if !canOpen && !enhancedPair {
		if err := p.state.emit(trace.Verbose, trace.Event{
			Phase: trace.Inline, RuleID: "commonmark.inline.emphasis.flanking", Decision: trace.Rejected, Span: p.input.span(start, start+run),
			Left: previousRuneString(p.input.data, start), Right: nextRuneString(p.input.data, start+run, end),
		}); err != nil {
			return 0, false, err
		}
		return start, false, nil
	}
	if !match.found {
		return p.recoverUnclosed(parent, start, end, match.openLength, delimiterKind(marker, match.openLength))
	}
	if enhancedPair {
		if err := p.state.emit(trace.Decisions, trace.Event{
			Phase: trace.Inline, RuleID: "enhance.inline.emphasis.paired-punctuation", Decision: trace.Accepted, Span: p.input.span(start, match.next),
		}); err != nil {
			return 0, false, err
		}
	}
	kind := delimiterKind(marker, match.openLength)
	node := p.state.builder.Add(kind, p.input.span(start, match.next))
	p.state.builder.AppendChild(parent, node)
	if err := p.parseRange(node, start+match.openLength, match.contentEnd); err != nil {
		return 0, false, err
	}
	return match.next, true, p.emitNode(node, kind)
}

type delimiterMatch struct {
	found      bool
	openLength int
	contentEnd int
	next       int
}

func (p *inlineParser) findClosing(start, end, openingRun int, marker byte) delimiterMatch {
	openLength := 1
	if marker == '~' {
		openLength = openingRun
	} else if openingRun%2 == 0 {
		openLength = 2
	}
	result := delimiterMatch{openLength: openLength}
	for candidate := start + openingRun; candidate < end; candidate++ {
		if p.input.data[candidate] == '\\' {
			candidate++
			continue
		}
		if p.input.data[candidate] != marker {
			continue
		}
		closingRun := byteRun(p.input.data, candidate, end, marker)
		if closingRun < openLength {
			candidate += closingRun - 1
			continue
		}
		_, canClose := delimiterFlanking(p.input.data, candidate, closingRun, marker, start, end)
		if !canClose {
			candidate += closingRun - 1
			continue
		}
		if marker != '~' {
			_, openCanClose := delimiterFlanking(p.input.data, start, openingRun, marker, 0, end)
			closeCanOpen, _ := delimiterFlanking(p.input.data, candidate, closingRun, marker, start, end)
			if openCanClose && closeCanOpen && (openingRun+closingRun)%3 == 0 && (openingRun%3 != 0 || closingRun%3 != 0) {
				candidate += closingRun - 1
				continue
			}
		}
		result.found = true
		if openingRun > 2 && openingRun == closingRun {
			result.contentEnd = candidate + closingRun - openLength
			result.next = candidate + closingRun
		} else {
			result.contentEnd = candidate
			result.next = candidate + openLength
		}
		return result
	}
	return result
}

func (p *inlineParser) recoverUnclosed(parent ast.NodeID, start, end, openingLength int, kind ast.Kind) (int, bool, error) {
	policy := p.state.parser.recovery[kind]
	switch policy {
	case Literal:
		return start, false, nil
	case Error:
		return 0, false, fmt.Errorf("parser: unclosed %s at byte %d", kind, p.input.offset(start))
	case RecoverAtParagraphEnd:
		if start+openingLength >= end {
			return start, false, nil
		}
		node := p.state.builder.Add(kind, p.input.span(start, end))
		p.state.builder.AppendChild(parent, node)
		if kind == ast.CodeSpan {
			p.state.builder.SetText(node, strings.ReplaceAll(string(p.input.data[start+openingLength:end]), "\n", " "))
		} else if err := p.parseRange(node, start+openingLength, end); err != nil {
			return 0, false, err
		}
		span := p.input.span(start, start+openingLength)
		p.state.addDiagnostic(diagnostic.Diagnostic{
			Code: "enhance.unclosed-inline", Severity: diagnostic.Warning, RuleID: "enhance.inline.recovery.paragraph-end", Span: span, Recovery: "paragraph-end",
			Fields: []diagnostic.Field{{Name: "kind", Value: kind.String()}},
		})
		if err := p.state.emit(trace.Decisions, trace.Event{
			Phase: trace.Inline, RuleID: "enhance.inline.recovery.paragraph-end", Decision: trace.Recovered, Span: span, NodeKind: kind,
			Fields: []trace.Field{{Name: "kind", Value: kind.String()}},
		}); err != nil {
			return 0, false, err
		}
		if err := p.emitNode(node, kind); err != nil {
			return 0, false, err
		}
		return end, true, nil
	default:
		return start, false, nil
	}
}

func delimiterKind(marker byte, length int) ast.Kind {
	if marker == '~' {
		return ast.Strikethrough
	}
	if length == 2 {
		return ast.Strong
	}
	return ast.Emphasis
}

func delimiterFlanking(data []byte, start, run int, marker byte, lower, upper int) (bool, bool) {
	previous, hasPrevious := previousRune(data, start, lower)
	next, hasNext := nextRune(data, start+run, upper)
	previousWhitespace := !hasPrevious || unicode.IsSpace(previous)
	nextWhitespace := !hasNext || unicode.IsSpace(next)
	previousPunctuation := hasPrevious && isUnicodePunctuation(previous)
	nextPunctuation := hasNext && isUnicodePunctuation(next)
	leftFlanking := !nextWhitespace && (!nextPunctuation || previousWhitespace || previousPunctuation)
	rightFlanking := !previousWhitespace && (!previousPunctuation || nextWhitespace || nextPunctuation)
	if marker == '_' {
		return leftFlanking && (!rightFlanking || previousPunctuation), rightFlanking && (!leftFlanking || nextPunctuation)
	}
	return leftFlanking, rightFlanking
}

func (p *inlineParser) isRangeSeparator(position, end int) bool {
	left, hasLeft := previousRune(p.input.data, position, 0)
	right, hasRight := nextRune(p.input.data, position+1, end)
	return hasLeft && hasRight && rangeOperand(left) && rangeOperand(right)
}

func rangeOperand(current rune) bool {
	return unicode.IsLetter(current) || unicode.IsDigit(current) || strings.ContainsRune("年月日時分秒개명번회층장권차주월일시분초", current)
}

var punctuationPairs = map[rune]rune{
	'"': '"', '\'': '\'', '(': ')', '[': ']', '{': '}',
	'“': '”', '‘': '’', '「': '」', '『': '』', '《': '》', '〈': '〉', '【': '】', '（': '）', '［': '］', '｛': '｝',
}

func pairedPunctuation(content []byte) bool {
	if len(content) < 2 {
		return false
	}
	first, firstSize := utf8.DecodeRune(content)
	last, lastSize := utf8.DecodeLastRune(content)
	if firstSize == 0 || lastSize == 0 {
		return false
	}
	return punctuationPairs[first] == last
}

func (p *inlineParser) parseLink(parent ast.NodeID, start, end int, image bool) (int, bool, error) {
	labelOpen := start
	if image {
		labelOpen++
	}
	labelClose := findClosingBracket(p.input.data, labelOpen+1, end)
	if labelClose < 0 {
		return start, false, p.unclosedStructural(start, ast.Image, image)
	}
	labelStart := labelOpen + 1
	label := string(p.input.data[labelStart:labelClose])
	next := labelClose + 1
	destination, title := "", ""
	matched := false
	if next < end && p.input.data[next] == '(' {
		closing := findLinkDestinationEnd(p.input.data, next+1, end)
		if closing >= 0 {
			rawDestination := string(p.input.data[next+1 : closing])
			destination, title = parseDestinationAndTitle(rawDestination)
			next = closing + 1
			matched = destination != "" || strings.TrimSpace(rawDestination) == ""
		}
	} else if next < end && p.input.data[next] == '[' {
		closing := findClosingBracket(p.input.data, next+1, end)
		if closing >= 0 {
			key := string(p.input.data[next+1 : closing])
			if key == "" {
				key = label
			}
			if reference, ok := p.state.references[normalizeReference(key)]; ok {
				destination, title, matched = reference.destination, reference.title, true
				next = closing + 1
			}
		}
	} else if reference, ok := p.state.references[normalizeReference(label)]; ok {
		destination, title, matched = reference.destination, reference.title, true
	}
	if !matched {
		return start, false, nil
	}
	kind := ast.Link
	if image {
		kind = ast.Image
	}
	node := p.state.builder.Add(kind, p.input.span(start, next))
	p.state.builder.SetDestination(node, unescapeBackslashes(destination))
	p.state.builder.SetTitle(node, title)
	p.state.builder.AppendChild(parent, node)
	if err := p.parseRange(node, labelStart, labelClose); err != nil {
		return 0, false, err
	}
	return next, true, p.emitNode(node, kind)
}

func (p *inlineParser) unclosedStructural(start int, kind ast.Kind, image bool) error {
	policy := p.state.parser.recovery[kind]
	if policy != Error {
		return nil
	}
	name := kind.String()
	if image {
		name = "image"
	}
	return fmt.Errorf("parser: unclosed %s label at byte %d", name, p.input.offset(start))
}

func (p *inlineParser) parseAngle(parent ast.NodeID, start, end int) (int, bool) {
	relative := bytes.IndexByte(p.input.data[start+1:end], '>')
	if relative < 0 {
		return start, false
	}
	closing := start + 1 + relative
	inside := string(p.input.data[start+1 : closing])
	if destination, ok := autolinkDestination(inside); ok {
		node := p.state.builder.Add(ast.Autolink, p.input.span(start, closing+1))
		p.state.builder.SetDestination(node, destination)
		p.state.builder.AppendChild(parent, node)
		p.appendLiteral(node, start+1, closing, inside)
		return closing + 1, true
	}
	if looksLikeInlineHTML(inside) {
		node := p.state.builder.Add(ast.RawHTML, p.input.span(start, closing+1))
		p.state.builder.SetContentSpan(node, p.input.span(start, closing+1))
		p.state.builder.AppendChild(parent, node)
		return closing + 1, true
	}
	return start, false
}

func autolinkDestination(inside string) (string, bool) {
	if strings.ContainsAny(inside, " \t\n<>") {
		return "", false
	}
	if strings.Contains(inside, "@") {
		if address, err := mail.ParseAddress(inside); err == nil && address.Address == inside {
			return "mailto:" + inside, true
		}
	}
	parsed, err := url.Parse(inside)
	if err == nil && parsed.Scheme != "" && validScheme(parsed.Scheme) {
		return inside, true
	}
	return "", false
}

func (p *inlineParser) parseEntity(parent ast.NodeID, start, end int) (int, bool) {
	limit := minInt(end, start+34)
	semicolon := bytes.IndexByte(p.input.data[start:limit], ';')
	if semicolon < 2 {
		return start, false
	}
	semicolon += start
	raw := string(p.input.data[start : semicolon+1])
	decoded := html.UnescapeString(raw)
	if decoded == raw {
		return start, false
	}
	p.appendLiteral(parent, start, semicolon+1, decoded)
	return semicolon + 1, true
}

func (p *inlineParser) parseExtendedAutolink(parent ast.NodeID, start, end int) (int, bool) {
	if start > 0 {
		previous, _ := previousRune(p.input.data, start, 0)
		if unicode.IsLetter(previous) || unicode.IsDigit(previous) || previous == '_' {
			return start, false
		}
	}
	remaining := p.input.data[start:end]
	var prefix []byte
	switch {
	case bytes.HasPrefix(remaining, []byte("https://")):
		prefix = []byte("https://")
	case bytes.HasPrefix(remaining, []byte("http://")):
		prefix = []byte("http://")
	case bytes.HasPrefix(remaining, []byte("www.")):
		prefix = []byte("www.")
	default:
		return start, false
	}
	length := len(prefix)
	for length < len(remaining) {
		current := remaining[length]
		if current <= ' ' || current == '<' {
			break
		}
		length++
	}
	for length > len(prefix) && strings.ContainsRune(".,:;!?", rune(remaining[length-1])) {
		length--
	}
	if length == len(prefix) {
		return start, false
	}
	visible := string(remaining[:length])
	destination := visible
	if bytes.Equal(prefix, []byte("www.")) {
		destination = "http://" + visible
	}
	node := p.state.builder.Add(ast.Autolink, p.input.span(start, start+length))
	p.state.builder.SetDestination(node, destination)
	p.state.builder.AppendChild(parent, node)
	p.appendSource(node, start, start+length)
	return start + length, true
}

func (p *inlineParser) appendSource(parent ast.NodeID, start, end int) {
	if start >= end {
		return
	}
	segmentStart := start
	for index := start; index <= end; index++ {
		if index == end || p.input.data[index] == '\n' {
			if segmentStart < index {
				node := p.state.builder.Add(ast.Text, p.input.span(segmentStart, index))
				if len(p.input.offsets) != 0 {
					p.state.builder.SetText(node, string(p.input.data[segmentStart:index]))
				}
				p.state.builder.AppendChild(parent, node)
			}
			if index < end {
				p.appendBreak(parent, index, false)
			}
			segmentStart = index + 1
		}
	}
}

func (p *inlineParser) appendLiteral(parent ast.NodeID, start, end int, literal string) {
	node := p.state.builder.Add(ast.Text, p.input.span(start, end))
	p.state.builder.SetText(node, literal)
	p.state.builder.AppendChild(parent, node)
}

func (p *inlineParser) appendBreak(parent ast.NodeID, position int, hard bool) {
	kind := ast.SoftBreak
	if hard {
		kind = ast.HardBreak
	}
	node := p.state.builder.Add(kind, p.input.span(position, minInt(position+1, len(p.input.data))))
	p.state.builder.AppendChild(parent, node)
}

func (p *inlineParser) emitDelimiterFound(start, run int, marker byte) error {
	if !p.state.tracing(trace.Verbose) {
		return nil
	}
	return p.state.emit(trace.Verbose, trace.Event{
		Phase: trace.Inline, RuleID: "inline.delimiter.found", Decision: trace.Observed, Span: p.input.span(start, start+run),
		Left: previousRuneString(p.input.data, start), Right: nextRuneString(p.input.data, start+run, len(p.input.data)),
		Fields: []trace.Field{{Name: "delimiter", Value: string(bytes.Repeat([]byte{marker}, run))}},
	})
}

func (p *inlineParser) emitNode(node ast.NodeID, kind ast.Kind) error {
	return p.state.emit(trace.Decisions, trace.Event{
		Phase: trace.Inline, RuleID: "inline.node.created", Decision: trace.Accepted, Span: p.state.builder.Document().Node(node).Span(), NodeKind: kind,
	})
}

func byteRun(data []byte, start, end int, marker byte) int {
	position := start
	for position < end && data[position] == marker {
		position++
	}
	return position - start
}

func previousRune(data []byte, position, lower int) (rune, bool) {
	if position <= lower {
		return 0, false
	}
	current, _ := utf8.DecodeLastRune(data[lower:position])
	return current, true
}

func nextRune(data []byte, position, upper int) (rune, bool) {
	if position >= upper {
		return 0, false
	}
	current, _ := utf8.DecodeRune(data[position:upper])
	return current, true
}

func previousRuneString(data []byte, position int) string {
	current, ok := previousRune(data, position, 0)
	if !ok {
		return ""
	}
	return string(current)
}

func nextRuneString(data []byte, position, upper int) string {
	current, ok := nextRune(data, position, upper)
	if !ok {
		return ""
	}
	return string(current)
}

func isUnicodePunctuation(current rune) bool {
	return unicode.IsPunct(current) || unicode.IsSymbol(current)
}

func isASCIIPunctuation(current byte) bool {
	return current >= '!' && current <= '/' || current >= ':' && current <= '@' || current >= '[' && current <= '`' || current >= '{' && current <= '~'
}

func findClosingBracket(data []byte, start, end int) int {
	depth := 0
	for position := start; position < end; position++ {
		switch data[position] {
		case '\\':
			position++
		case '[':
			depth++
		case ']':
			if depth == 0 {
				return position
			}
			depth--
		}
	}
	return -1
}

func findLinkDestinationEnd(data []byte, start, end int) int {
	depth := 0
	quote := byte(0)
	for position := start; position < end; position++ {
		current := data[position]
		if current == '\\' {
			position++
			continue
		}
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '"' || current == '\'' {
			quote = current
			continue
		}
		switch current {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return position
			}
			depth--
		case '\n':
			return -1
		}
	}
	return -1
}

func parseDestinationAndTitle(value string) (string, string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ""
	}
	if trimmed[0] == '<' {
		if closing := strings.IndexByte(trimmed, '>'); closing > 0 {
			destination := trimmed[1:closing]
			return destination, strings.Trim(strings.TrimSpace(trimmed[closing+1:]), "\"'()")
		}
	}
	fields := strings.Fields(trimmed)
	destination := fields[0]
	title := ""
	if len(fields) > 1 {
		title = strings.Trim(strings.Join(fields[1:], " "), "\"'()")
	}
	return destination, title
}

func unescapeBackslashes(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for index := 0; index < len(value); index++ {
		if value[index] == '\\' && index+1 < len(value) && isASCIIPunctuation(value[index+1]) {
			index++
		}
		builder.WriteByte(value[index])
	}
	return builder.String()
}

func validScheme(scheme string) bool {
	if scheme == "" || !isASCIIAlpha(scheme[0]) {
		return false
	}
	for index := 1; index < len(scheme); index++ {
		current := scheme[index]
		if !isASCIIAlpha(current) && (current < '0' || current > '9') && current != '+' && current != '.' && current != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlpha(current byte) bool {
	return current >= 'a' && current <= 'z' || current >= 'A' && current <= 'Z'
}

func looksLikeInlineHTML(inside string) bool {
	if inside == "" {
		return false
	}
	if strings.HasPrefix(inside, "!--") || strings.HasPrefix(inside, "?") || strings.HasPrefix(inside, "!") {
		return true
	}
	trimmed := strings.TrimPrefix(inside, "/")
	return trimmed != "" && isASCIIAlpha(trimmed[0])
}
