package parser

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/profile"
)

type sourceLine struct {
	start, end, newlineEnd   int
	contentStart, contentEnd int
	lazy                     bool
	logical                  []byte
	offsets                  []int
}

func scanLines(source []byte) []sourceLine {
	if len(source) == 0 {
		return nil
	}
	lines := make([]sourceLine, 0, bytes.Count(source, []byte{'\n'})+1)
	for start := 0; start < len(source); {
		newline := bytes.IndexByte(source[start:], '\n')
		newlineEnd := len(source)
		end := len(source)
		if newline >= 0 {
			end = start + newline
			newlineEnd = end + 1
		}
		if end > start && source[end-1] == '\r' {
			end--
		}
		lines = append(lines, sourceLine{start: start, end: end, newlineEnd: newlineEnd, contentStart: start, contentEnd: end})
		start = newlineEnd
	}
	return lines
}

func (s *parseState) parseBlocks(lines []sourceLine, parent ast.NodeID) error {
	for index := 0; index < len(lines); {
		if err := s.checkContext(); err != nil {
			return err
		}
		line := lines[index]
		content := s.lineBytes(line)
		if isBlank(content) {
			index++
			continue
		}
		if marker, info, ok := fenceStart(content); ok {
			next, err := s.parseFence(lines, index, parent, marker, info)
			if err != nil {
				return err
			}
			index = next
			continue
		}
		if level, start, end, ok := atxHeading(content); ok {
			node := s.builder.Add(ast.Heading, ast.Span{Start: line.contentStart, End: line.contentEnd})
			s.builder.SetIntegers(node, level, 0)
			s.builder.AppendChild(parent, node)
			s.inlines = append(s.inlines, inlineBlock{node: node, spans: []ast.Span{{Start: lineSourceOffset(line, start), End: lineSourceOffset(line, end)}}})
			index++
			continue
		}
		if isThematicBreak(content) {
			node := s.builder.Add(ast.ThematicBreak, ast.Span{Start: line.contentStart, End: line.contentEnd})
			s.builder.AppendChild(parent, node)
			index++
			continue
		}
		if quote, ok := stripBlockQuote(line, s.source); ok {
			start := index
			quoted := []sourceLine{quote}
			index++
			for index < len(lines) {
				adjusted, matched := stripBlockQuote(lines[index], s.source)
				if matched {
					quoted = append(quoted, adjusted)
					index++
					continue
				}
				// CommonMark permits an unmarked lazy continuation while the
				// current quote paragraph can consume the line. A new block or
				// blank line ends that continuation.
				candidate := s.lineBytes(lines[index])
				if isBlank(candidate) || startsBlock(candidate) || !s.quoteAllowsLazyContinuation(quoted) {
					break
				}
				lazy := lines[index]
				lazy.lazy = true
				quoted = append(quoted, lazy)
				index++
			}
			node := s.builder.Add(ast.BlockQuote, ast.Span{Start: lines[start].start, End: lines[index-1].newlineEnd})
			s.builder.AppendChild(parent, node)
			if err := s.parseBlocks(quoted, node); err != nil {
				return err
			}
			continue
		}
		if marker, ok := parseListMarker(content); ok {
			next, err := s.parseList(lines, index, parent, marker)
			if err != nil {
				return err
			}
			index = next
			continue
		}
		if indentWidth(content) >= 4 {
			start := index
			var codeLines []string
			for index < len(lines) {
				candidate := s.lineBytes(lines[index])
				if !isBlank(candidate) && indentWidth(candidate) < 4 {
					break
				}
				if isBlank(candidate) {
					codeLines = append(codeLines, stripIndent(candidate, minInt(4, indentWidth(candidate))))
				} else {
					codeLines = append(codeLines, stripIndent(candidate, 4))
				}
				index++
			}
			for len(codeLines) > 0 && codeLines[len(codeLines)-1] == "" {
				codeLines = codeLines[:len(codeLines)-1]
			}
			node := s.builder.Add(ast.CodeBlock, ast.Span{Start: lines[start].start, End: lines[index-1].newlineEnd})
			s.builder.SetText(node, strings.Join(codeLines, "\n")+"\n")
			s.builder.AppendChild(parent, node)
			continue
		}
		if rule, ok := htmlBlockStart(content, false); ok {
			start := index
			index++
			if !htmlBlockEnded(content, rule) {
				for index < len(lines) {
					candidate := s.lineBytes(lines[index])
					if rule.untilBlank && isBlank(candidate) {
						break
					}
					index++
					if htmlBlockEnded(candidate, rule) {
						break
					}
				}
			}
			node := s.builder.Add(ast.HTMLBlock, ast.Span{Start: lines[start].start, End: lines[index-1].newlineEnd})
			s.builder.SetText(node, s.htmlBlockText(lines[start:index]))
			s.builder.AppendChild(parent, node)
			continue
		}
		if definition, next, ok := s.parseReferenceDefinition(lines, index); ok {
			key := normalizeReference(definition.label)
			if _, exists := s.references[key]; !exists {
				s.references[key] = reference{destination: definition.destination, title: definition.title}
			}
			index = next
			continue
		}
		next, err := s.parseParagraph(lines, index, parent)
		if err != nil {
			return err
		}
		index = next
	}
	return nil
}

func (s *parseState) htmlBlockText(lines []sourceLine) string {
	var output strings.Builder
	for index, line := range lines {
		output.Write(s.lineBytes(line))
		if index+1 < len(lines) || line.newlineEnd > line.end {
			output.WriteByte('\n')
		}
	}
	return output.String()
}

func (s *parseState) quoteAllowsLazyContinuation(lines []sourceLine) bool {
	if len(lines) == 0 || isBlank(s.lineBytes(lines[len(lines)-1])) {
		return false
	}
	var openFence string
	for _, line := range lines {
		leaf := quoteLeafContent(s.lineBytes(line))
		if openFence != "" {
			if isClosingFence(leaf, openFence) {
				openFence = ""
			}
			continue
		}
		if marker, _, ok := fenceStart(leaf); ok {
			openFence = marker
		}
	}
	if openFence != "" {
		return false
	}
	leaf := quoteLeafContent(s.lineBytes(lines[len(lines)-1]))
	if isBlank(leaf) || indentWidth(leaf) >= 4 || startsBlock(leaf) {
		return false
	}
	_, _, _, definition := referenceDefinition(leaf)
	return !definition
}

func quoteLeafContent(content []byte) []byte {
	for {
		line := sourceLine{contentStart: 0, contentEnd: len(content)}
		if stripped, ok := stripBlockQuote(line, content); ok {
			content = content[stripped.contentStart:stripped.contentEnd]
			continue
		}
		if marker, ok := parseListMarker(content); ok && marker.offset < len(content) {
			content = content[marker.offset:]
			continue
		}
		return content
	}
}

func (s *parseState) parseParagraph(lines []sourceLine, start int, parent ast.NodeID) (int, error) {
	index := start
	var spans []ast.Span
	for index < len(lines) {
		content := s.lineBytes(lines[index])
		if isBlank(content) {
			break
		}
		if index > start {
			if !lines[index].lazy {
				if _, _, ok := setextUnderline(content); ok {
					break
				}
				if s.parser.profile.Has(profile.Tables) && isTableDelimiter(content) {
					break
				}
				if startsBlock(content) {
					break
				}
			}
		}
		logicalStart := len(content) - len(bytes.TrimLeft(content, " \t"))
		spans = append(spans, ast.Span{Start: lineSourceOffset(lines[index], logicalStart), End: lineSourceOffset(lines[index], len(content))})
		index++
	}
	if len(spans) > 0 && index < len(lines) {
		if level, _, ok := setextUnderline(s.lineBytes(lines[index])); ok {
			last := &spans[len(spans)-1]
			contentLength := last.End - last.Start
			trimmedLength := len(bytes.TrimRight(s.source[last.Start:last.End], " \t"))
			last.End -= contentLength - trimmedLength
			node := s.builder.Add(ast.Heading, ast.Span{Start: lines[start].start, End: lines[index].newlineEnd})
			s.builder.SetIntegers(node, level, 0)
			s.builder.AppendChild(parent, node)
			s.inlines = append(s.inlines, inlineBlock{node: node, spans: spans})
			return index + 1, nil
		}
		if s.parser.profile.Has(profile.Tables) && isTableDelimiter(s.lineBytes(lines[index])) {
			return s.parseTable(lines, start, index, parent)
		}
	}
	if len(spans) > 0 {
		last := &spans[len(spans)-1]
		last.End = last.Start + len(bytes.TrimRight(s.source[last.Start:last.End], " \t"))
	}
	node := s.builder.Add(ast.Paragraph, ast.Span{Start: lines[start].start, End: lines[index-1].newlineEnd})
	s.builder.AppendChild(parent, node)
	s.inlines = append(s.inlines, inlineBlock{node: node, spans: spans})
	return index, nil
}

func (s *parseState) parseFence(lines []sourceLine, start int, parent ast.NodeID, marker string, info string) (int, error) {
	index := start + 1
	openingIndent := indentWidth(s.lineBytes(lines[start]))
	var codeLines []string
	for index < len(lines) {
		content := s.lineBytes(lines[index])
		if isClosingFence(content, marker) {
			index++
			break
		}
		remove := minInt(indentWidth(content), openingIndent)
		if remove == 0 {
			codeLines = append(codeLines, string(content))
		} else {
			codeLines = append(codeLines, stripIndent(content, remove))
		}
		index++
	}
	end := lines[len(lines)-1].newlineEnd
	if index > 0 && index <= len(lines) {
		end = lines[index-1].newlineEnd
	}
	node := s.builder.Add(ast.CodeBlock, ast.Span{Start: lines[start].start, End: end})
	s.builder.SetText(node, strings.Join(codeLines, "\n")+func() string {
		if len(codeLines) > 0 {
			return "\n"
		}
		return ""
	}())
	s.builder.SetTitle(node, decodeLinkText(info))
	s.builder.SetIntegers(node, int(marker[0]), len(marker))
	s.builder.AppendChild(parent, node)
	return index, nil
}

func isClosingFence(line []byte, opening string) bool {
	if indentWidth(line) > 3 {
		return false
	}
	trimmed := bytes.TrimLeft(line, " \t")
	run := 0
	for run < len(trimmed) && trimmed[run] == opening[0] {
		run++
	}
	return run >= len(opening) && len(bytes.Trim(trimmed[run:], " \t")) == 0
}

type listMarker struct {
	ordered   bool
	start     int
	indent    int
	width     int
	offset    int
	markerEnd int
	delimiter byte
}

func (s *parseState) parseList(lines []sourceLine, start int, parent ast.NodeID, first listMarker) (int, error) {
	index := start
	list := s.builder.Add(ast.List, ast.Span{Start: lines[start].start, End: lines[start].newlineEnd})
	s.builder.SetIntegers(list, first.start, int(first.delimiter))
	s.builder.AppendChild(parent, list)
	loose := false
	for index < len(lines) {
		if index > start && isThematicBreak(s.lineBytes(lines[index])) {
			break
		}
		marker, ok := parseListMarker(s.lineBytes(lines[index]))
		if !ok || marker.ordered != first.ordered || marker.delimiter != first.delimiter {
			break
		}
		itemStart := index
		adjusted := stripLineContainer(lines[index], s.source, marker.markerEnd, marker.width)
		itemLines := []sourceLine{adjusted}
		itemInitiallyEmpty := len(s.lineBytes(adjusted)) == 0
		openFence := ""
		if fence, _, fenced := fenceStart(s.lineBytes(adjusted)); fenced {
			openFence = fence
		}
		index++
		blankPending := false
		for index < len(lines) {
			candidate := s.lineBytes(lines[index])
			if next, matched := parseListMarker(candidate); matched && next.indent < marker.width {
				if next.ordered == first.ordered && next.delimiter == first.delimiter && blankPending {
					loose = true
				}
				break
			}
			if isBlank(candidate) {
				itemLines = append(itemLines, lines[index])
				if openFence == "" {
					blankPending = true
				}
				index++
				continue
			}
			indent := indentWidth(candidate)
			if blankPending && itemInitiallyEmpty || indent < marker.width && (blankPending || startsBlock(candidate)) {
				break
			}
			remove := minInt(indent, marker.width)
			continuation := stripLineContainer(lines[index], s.source, 0, remove)
			if indent < marker.width {
				continuation.lazy = true
			}
			itemLines = append(itemLines, continuation)
			if blankPending && indent <= marker.width {
				loose = true
			}
			if blankPending && indent <= marker.width {
				trimmed := s.lineBytes(continuation)
				if _, _, _, definition := referenceDefinition(trimmed); definition {
					loose = true
				}
			}
			blankPending = false
			adjustedContent := s.lineBytes(continuation)
			if openFence != "" {
				if isClosingFence(adjustedContent, openFence) {
					openFence = ""
				}
			} else if fence, _, fenced := fenceStart(adjustedContent); fenced {
				openFence = fence
			}
			index++
		}
		item := s.builder.Add(ast.ListItem, ast.Span{Start: lines[itemStart].start, End: lines[index-1].newlineEnd})
		s.builder.AppendChild(list, item)
		if s.parser.profile.Has(profile.TaskLists) {
			firstContent := s.lineBytes(itemLines[0])
			if checked, consumed, ok := taskMarker(firstContent); ok {
				check := s.builder.Add(ast.TaskCheck, ast.Span{Start: lineSourceOffset(itemLines[0], 0), End: lineSourceOffset(itemLines[0], consumed)})
				if checked {
					s.builder.SetFlags(check, ast.TaskChecked)
				}
				s.builder.AppendChild(item, check)
				itemLines[0] = trimLinePrefix(itemLines[0], consumed)
			}
		}
		if err := s.parseBlocks(itemLines, item); err != nil {
			return 0, err
		}
		if listItemIsLoose(s.builder.Document(), s.source, item) {
			loose = true
		}
	}
	flags := uint32(0)
	if first.ordered {
		flags |= ast.ListOrdered
	}
	if !loose {
		flags |= ast.ListTight
	}
	s.builder.SetFlags(list, flags)
	// The arena stores immutable spans, so the list span is represented by its
	// child item spans during rendering and source mapping.
	return index, nil
}

func listItemIsLoose(document *ast.Document, source []byte, item ast.NodeID) bool {
	previous := ast.NoNode
	for child := document.Node(item).FirstChild(); child != ast.NoNode; child = document.Node(child).NextSibling() {
		if previous != ast.NoNode {
			left := document.Node(previous).Span().End
			right := document.Node(child).Span().Start
			if left < right && right <= len(source) {
				gap := source[left:right]
				if bytes.Count(gap, []byte{'\n'}) > 0 && len(bytes.TrimSpace(gap)) == 0 {
					return true
				}
			}
		}
		previous = child
	}
	return false
}

func (s *parseState) parseTable(lines []sourceLine, headerIndex, delimiterIndex int, parent ast.NodeID) (int, error) {
	headerCells := splitTableCells(s.lineBytes(lines[headerIndex]), lines[headerIndex].contentStart)
	delimiterCells := splitTableCells(s.lineBytes(lines[delimiterIndex]), lines[delimiterIndex].contentStart)
	if len(headerCells) == 0 || len(headerCells) != len(delimiterCells) {
		node := s.builder.Add(ast.Paragraph, ast.Span{Start: lines[headerIndex].start, End: lines[headerIndex].newlineEnd})
		s.builder.AppendChild(parent, node)
		s.inlines = append(s.inlines, inlineBlock{node: node, spans: []ast.Span{{Start: lines[headerIndex].contentStart, End: lines[headerIndex].contentEnd}}})
		return delimiterIndex, nil
	}
	table := s.builder.Add(ast.Table, ast.Span{Start: lines[headerIndex].start, End: lines[delimiterIndex].newlineEnd})
	s.builder.AppendChild(parent, table)
	head := s.builder.Add(ast.TableHead, ast.Span{Start: lines[headerIndex].start, End: lines[delimiterIndex].newlineEnd})
	s.builder.AppendChild(table, head)
	row := s.builder.Add(ast.TableRow, ast.Span{Start: lines[headerIndex].contentStart, End: lines[headerIndex].contentEnd})
	s.builder.AppendChild(head, row)
	for cellIndex, span := range headerCells {
		cell := s.builder.Add(ast.TableCell, span)
		s.builder.SetFlags(cell, tableAlignment(s.source[delimiterCells[cellIndex].Start:delimiterCells[cellIndex].End]))
		s.builder.AppendChild(row, cell)
		s.inlines = append(s.inlines, inlineBlock{node: cell, spans: []ast.Span{span}})
	}
	body := s.builder.Add(ast.TableBody, ast.Span{Start: lines[delimiterIndex].newlineEnd, End: lines[delimiterIndex].newlineEnd})
	s.builder.AppendChild(table, body)
	index := delimiterIndex + 1
	for index < len(lines) && !isBlank(s.lineBytes(lines[index])) && bytes.Contains(s.lineBytes(lines[index]), []byte{'|'}) {
		cells := splitTableCells(s.lineBytes(lines[index]), lines[index].contentStart)
		bodyRow := s.builder.Add(ast.TableRow, ast.Span{Start: lines[index].contentStart, End: lines[index].contentEnd})
		s.builder.AppendChild(body, bodyRow)
		for cellIndex := 0; cellIndex < len(headerCells); cellIndex++ {
			span := ast.Span{Start: lines[index].contentEnd, End: lines[index].contentEnd}
			if cellIndex < len(cells) {
				span = cells[cellIndex]
			}
			cell := s.builder.Add(ast.TableCell, span)
			s.builder.SetFlags(cell, tableAlignment(s.source[delimiterCells[cellIndex].Start:delimiterCells[cellIndex].End]))
			s.builder.AppendChild(bodyRow, cell)
			s.inlines = append(s.inlines, inlineBlock{node: cell, spans: []ast.Span{span}})
		}
		index++
	}
	return index, nil
}

func (s *parseState) lineBytes(line sourceLine) []byte {
	if line.logical != nil {
		return line.logical
	}
	return s.source[line.contentStart:line.contentEnd]
}

func lineSourceOffset(line sourceLine, logical int) int {
	if line.offsets == nil {
		return line.contentStart + logical
	}
	if logical < 0 {
		logical = 0
	}
	if logical >= len(line.offsets) {
		return line.offsets[len(line.offsets)-1]
	}
	return line.offsets[logical]
}

func trimLinePrefix(line sourceLine, count int) sourceLine {
	if count <= 0 {
		return line
	}
	if line.logical == nil {
		line.contentStart += count
		return line
	}
	if count > len(line.logical) {
		count = len(line.logical)
	}
	line.logical = line.logical[count:]
	line.offsets = line.offsets[count:]
	line.contentStart = line.offsets[0]
	line.contentEnd = line.offsets[len(line.offsets)-1]
	return line
}

// stripLineContainer removes a block quote or list container by visual
// columns. Tabs in the consumed indentation may straddle the boundary, so the
// remaining columns are materialized as spaces while offsets keep mapping the
// logical view back to the original source bytes.
func stripLineContainer(line sourceLine, source []byte, markerEnd, containerWidth int) sourceLine {
	content := source[line.contentStart:line.contentEnd]
	if line.logical != nil {
		content = line.logical
	}
	prefixEnd := markerEnd
	for prefixEnd < len(content) && (content[prefixEnd] == ' ' || content[prefixEnd] == '\t') {
		prefixEnd++
	}
	column := 0
	for _, current := range content[:prefixEnd] {
		if current == '\t' {
			column += 4 - column%4
		} else {
			column++
		}
	}
	remaining := column - containerWidth
	if remaining < 0 {
		remaining = 0
	}
	logical := make([]byte, remaining+len(content)-prefixEnd)
	for index := 0; index < remaining; index++ {
		logical[index] = ' '
	}
	copy(logical[remaining:], content[prefixEnd:])
	oldOffsets := line.offsets
	if oldOffsets == nil {
		oldOffsets = make([]int, len(content)+1)
		for index := range oldOffsets {
			oldOffsets[index] = line.contentStart + index
		}
	}
	offsets := make([]int, len(logical)+1)
	indentSourceStart := oldOffsets[minInt(markerEnd, len(content))]
	indentSourceEnd := oldOffsets[prefixEnd]
	if remaining == 0 {
		offsets[0] = indentSourceEnd
	} else {
		for index := 0; index <= remaining; index++ {
			offsets[index] = indentSourceStart + (indentSourceEnd-indentSourceStart)*index/remaining
		}
	}
	for index := 1; index <= len(content)-prefixEnd; index++ {
		offsets[remaining+index] = oldOffsets[prefixEnd+index]
	}
	line.logical = logical
	line.offsets = offsets
	line.contentStart = offsets[0]
	line.contentEnd = offsets[len(offsets)-1]
	return line
}

func isBlank(line []byte) bool { return len(bytes.TrimSpace(line)) == 0 }

func indentWidth(line []byte) int {
	width := 0
	for _, current := range line {
		switch current {
		case ' ':
			width++
		case '\t':
			width += 4 - width%4
		default:
			return width
		}
	}
	return width
}

func byteOffsetForIndent(line []byte, wanted int) int {
	width := 0
	for index, current := range line {
		if current != ' ' && current != '\t' || width >= wanted {
			return index
		}
		if current == ' ' {
			width++
		} else {
			width += 4 - width%4
		}
	}
	return len(line)
}

func stripIndent(line []byte, wanted int) string {
	width := 0
	for index, current := range line {
		switch current {
		case ' ':
			width++
		case '\t':
			next := width + 4 - width%4
			if next > wanted {
				return strings.Repeat(" ", next-wanted) + string(line[index+1:])
			}
			width = next
		default:
			return string(line[index:])
		}
		if width >= wanted {
			return string(line[index+1:])
		}
	}
	return ""
}

func atxHeading(line []byte) (level, start, end int, ok bool) {
	if indentWidth(line) > 3 {
		return 0, 0, 0, false
	}
	indent := byteOffsetForIndent(line, 3)
	index := indent
	for index < len(line) && line[index] == '#' && index-indent < 6 {
		index++
	}
	level = index - indent
	if level == 0 || index < len(line) && line[index] != ' ' && line[index] != '\t' {
		return 0, 0, 0, false
	}
	for index < len(line) && (line[index] == ' ' || line[index] == '\t') {
		index++
	}
	end = len(line)
	trimmed := bytes.TrimRight(line[index:end], " \t")
	end = index + len(trimmed)
	closing := end
	for closing > index && line[closing-1] == '#' {
		closing--
	}
	if closing < end && (closing == index || line[closing-1] == ' ' || line[closing-1] == '\t') {
		end = len(bytes.TrimRight(line[index:closing], " \t")) + index
	}
	return level, index, end, true
}

func setextUnderline(line []byte) (level int, marker byte, ok bool) {
	if indentWidth(line) > 3 {
		return 0, 0, false
	}
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return 0, 0, false
	}
	marker = trimmed[0]
	if marker != '=' && marker != '-' || !allByte(trimmed, marker) {
		return 0, 0, false
	}
	if marker == '=' {
		return 1, marker, true
	}
	return 2, marker, true
}

func isThematicBreak(line []byte) bool {
	if indentWidth(line) > 3 {
		return false
	}
	trimmed := bytes.TrimSpace(line)
	count := 0
	var marker byte
	for _, current := range trimmed {
		if current == ' ' || current == '\t' {
			continue
		}
		if marker == 0 {
			marker = current
		}
		if current != marker || marker != '*' && marker != '-' && marker != '_' {
			return false
		}
		count++
	}
	return count >= 3
}

func fenceStart(line []byte) (marker, info string, ok bool) {
	if indentWidth(line) > 3 {
		return "", "", false
	}
	trimmed := bytes.TrimLeft(line, " \t")
	if len(trimmed) < 3 || trimmed[0] != '`' && trimmed[0] != '~' {
		return "", "", false
	}
	length := 0
	for length < len(trimmed) && trimmed[length] == trimmed[0] {
		length++
	}
	if length < 3 {
		return "", "", false
	}
	info = strings.TrimSpace(string(trimmed[length:]))
	if trimmed[0] == '`' && strings.ContainsRune(info, '`') {
		return "", "", false
	}
	return string(trimmed[:length]), info, true
}

func stripBlockQuote(line sourceLine, source []byte) (sourceLine, bool) {
	content := source[line.contentStart:line.contentEnd]
	if line.logical != nil {
		content = line.logical
	}
	index := byteOffsetForIndent(content, 3)
	if index >= len(content) || content[index] != '>' {
		return sourceLine{}, false
	}
	markerEnd := index + 1
	containerWidth := index + 1
	if markerEnd < len(content) && (content[markerEnd] == ' ' || content[markerEnd] == '\t') {
		containerWidth++
	}
	return stripLineContainer(line, source, markerEnd, containerWidth), true
}

func parseListMarker(line []byte) (listMarker, bool) {
	if indentWidth(line) > 3 {
		return listMarker{}, false
	}
	indent := byteOffsetForIndent(line, 3)
	if indent >= len(line) {
		return listMarker{}, false
	}
	if line[indent] == '-' || line[indent] == '+' || line[indent] == '*' {
		markerEnd := indent + 1
		padding, offset, ok := listMarkerPadding(line, markerEnd, indent+1)
		if !ok {
			return listMarker{}, false
		}
		return listMarker{indent: indent, width: indent + 1 + padding, offset: offset, markerEnd: markerEnd, delimiter: line[indent]}, true
	}
	index := indent
	for index < len(line) && index-indent < 9 && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	if index == indent || index >= len(line) || line[index] != '.' && line[index] != ')' {
		return listMarker{}, false
	}
	markerEnd := index + 1
	padding, offset, ok := listMarkerPadding(line, markerEnd, markerEnd)
	if !ok {
		return listMarker{}, false
	}
	start, err := strconv.Atoi(string(line[indent:index]))
	if err != nil {
		return listMarker{}, false
	}
	return listMarker{ordered: true, start: start, indent: indent, width: markerEnd + padding, offset: offset, markerEnd: markerEnd, delimiter: line[index]}, true
}

func listMarkerPadding(line []byte, markerEnd, markerColumns int) (padding, offset int, ok bool) {
	if markerEnd == len(line) {
		return 1, markerEnd, true
	}
	if line[markerEnd] != ' ' && line[markerEnd] != '\t' {
		return 0, 0, false
	}
	position := markerEnd
	column := markerColumns
	for position < len(line) && (line[position] == ' ' || line[position] == '\t') {
		if line[position] == ' ' {
			column++
		} else {
			column += 4 - column%4
		}
		position++
	}
	padding = column - markerColumns
	if padding >= 1 && padding <= 4 {
		return padding, position, true
	}
	return 1, markerEnd + 1, true
}

func taskMarker(line []byte) (checked bool, consumed int, ok bool) {
	if len(line) < 4 || line[0] != '[' || line[2] != ']' || line[3] != ' ' {
		return false, 0, false
	}
	if line[1] != ' ' && line[1] != 'x' && line[1] != 'X' {
		return false, 0, false
	}
	return line[1] == 'x' || line[1] == 'X', 4, true
}

func startsBlock(line []byte) bool {
	if _, _, _, ok := atxHeading(line); ok {
		return true
	}
	if _, _, ok := fenceStart(line); ok || isThematicBreak(line) {
		return true
	}
	if _, ok := htmlBlockStart(line, true); ok {
		return true
	}
	if marker, ok := parseListMarker(line); ok {
		return marker.offset < len(line) && (!marker.ordered || marker.start == 1)
	}
	trimmed := bytes.TrimLeft(line, " \t")
	return len(trimmed) > 0 && trimmed[0] == '>'
}

var rawHTMLBlockTags = map[string]bool{
	"pre": true, "script": true, "style": true, "textarea": true,
}

var commonMarkBlockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "base": true, "basefont": true, "blockquote": true,
	"body": true, "caption": true, "center": true, "col": true, "colgroup": true, "dd": true, "details": true,
	"dialog": true, "dir": true, "div": true, "dl": true, "dt": true, "fieldset": true, "figcaption": true,
	"figure": true, "footer": true, "form": true, "frame": true, "frameset": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "head": true, "header": true, "hr": true, "html": true,
	"iframe": true, "legend": true, "li": true, "link": true, "main": true, "menu": true, "menuitem": true,
	"nav": true, "noframes": true, "ol": true, "optgroup": true, "option": true, "p": true, "param": true,
	"search": true, "section": true, "source": true, "summary": true, "table": true, "tbody": true, "td": true, "tfoot": true,
	"th": true, "thead": true, "title": true, "tr": true, "track": true, "ul": true,
}

func referenceDefinition(line []byte) (destination, title, label string, ok bool) {
	parsed, parsedOK := parseReferenceDefinitionText(append(append([]byte(nil), line...), '\n'))
	if !parsedOK {
		return "", "", "", false
	}
	return parsed.destination, parsed.title, parsed.label, true
}

func normalizeReference(label string) string {
	label = strings.ToLower(strings.Join(strings.Fields(label), " "))
	// Unicode default case folding expands sharp s, while strings.ToLower
	// intentionally performs only a one-rune mapping.
	return strings.ReplaceAll(label, "ß", "ss")
}

func isTableDelimiter(line []byte) bool {
	cells := splitTableCells(line, 0)
	if len(cells) == 0 {
		return false
	}
	for _, span := range cells {
		cell := bytes.TrimSpace(line[span.Start:span.End])
		cell = bytes.TrimPrefix(cell, []byte{':'})
		cell = bytes.TrimSuffix(cell, []byte{':'})
		if len(cell) < 3 || !allByte(cell, '-') {
			return false
		}
	}
	return true
}

func splitTableCells(line []byte, base int) []ast.Span {
	start := 0
	end := len(line)
	if start < end && line[start] == '|' {
		start++
	}
	if start < end && line[end-1] == '|' && (end < 2 || line[end-2] != '\\') {
		end--
	}
	var cells []ast.Span
	cellStart := start
	escaped := false
	for index := start; index <= end; index++ {
		if index < end && line[index] == '\\' && !escaped {
			escaped = true
			continue
		}
		if index == end || line[index] == '|' && !escaped {
			left := cellStart
			right := index
			for left < right && (line[left] == ' ' || line[left] == '\t') {
				left++
			}
			for right > left && (line[right-1] == ' ' || line[right-1] == '\t') {
				right--
			}
			cells = append(cells, ast.Span{Start: base + left, End: base + right})
			cellStart = index + 1
		}
		escaped = false
	}
	return cells
}

func tableAlignment(cell []byte) uint32 {
	trimmed := bytes.TrimSpace(cell)
	left := len(trimmed) > 0 && trimmed[0] == ':'
	right := len(trimmed) > 0 && trimmed[len(trimmed)-1] == ':'
	switch {
	case left && right:
		return ast.TableAlignCenter
	case left:
		return ast.TableAlignLeft
	case right:
		return ast.TableAlignRight
	default:
		return 0
	}
}

func allByte(value []byte, expected byte) bool {
	for _, current := range value {
		if current != expected {
			return false
		}
	}
	return true
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
