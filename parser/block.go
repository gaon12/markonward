package parser

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/profile"
)

type sourceLine struct {
	start, end, newlineEnd   int
	contentStart, contentEnd int
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
			s.inlines = append(s.inlines, inlineBlock{node: node, spans: []ast.Span{{Start: line.contentStart + start, End: line.contentStart + end}}})
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
				if !matched {
					break
				}
				quoted = append(quoted, adjusted)
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
			var spans []ast.Span
			for index < len(lines) {
				candidate := s.lineBytes(lines[index])
				if !isBlank(candidate) && indentWidth(candidate) < 4 {
					break
				}
				contentStart := lines[index].contentStart
				if !isBlank(candidate) {
					contentStart += byteOffsetForIndent(candidate, 4)
				}
				spans = append(spans, ast.Span{Start: contentStart, End: lines[index].contentEnd})
				index++
			}
			node := s.builder.Add(ast.CodeBlock, ast.Span{Start: lines[start].start, End: lines[index-1].newlineEnd})
			s.builder.SetText(node, joinSpans(s.source, spans, "\n")+"\n")
			s.builder.AppendChild(parent, node)
			continue
		}
		if looksLikeHTMLBlock(content) {
			start := index
			index++
			for index < len(lines) && !isBlank(s.lineBytes(lines[index])) {
				index++
			}
			node := s.builder.Add(ast.HTMLBlock, ast.Span{Start: lines[start].start, End: lines[index-1].newlineEnd})
			s.builder.SetContentSpan(node, ast.Span{Start: lines[start].contentStart, End: lines[index-1].contentEnd})
			s.builder.AppendChild(parent, node)
			continue
		}
		if destination, title, label, ok := referenceDefinition(content); ok {
			s.references[normalizeReference(label)] = reference{destination: destination, title: title}
			index++
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

func (s *parseState) parseParagraph(lines []sourceLine, start int, parent ast.NodeID) (int, error) {
	index := start
	var spans []ast.Span
	for index < len(lines) {
		content := s.lineBytes(lines[index])
		if isBlank(content) || index > start && startsBlock(content) {
			break
		}
		if index > start {
			if _, _, ok := setextUnderline(content); ok {
				break
			}
			if s.parser.profile.Has(profile.Tables) && isTableDelimiter(content) {
				break
			}
		}
		spans = append(spans, ast.Span{Start: lines[index].contentStart, End: lines[index].contentEnd})
		index++
	}
	if len(spans) == 1 && index < len(lines) {
		if level, _, ok := setextUnderline(s.lineBytes(lines[index])); ok {
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
	node := s.builder.Add(ast.Paragraph, ast.Span{Start: lines[start].start, End: lines[index-1].newlineEnd})
	s.builder.AppendChild(parent, node)
	s.inlines = append(s.inlines, inlineBlock{node: node, spans: spans})
	return index, nil
}

func (s *parseState) parseFence(lines []sourceLine, start int, parent ast.NodeID, marker string, info string) (int, error) {
	index := start + 1
	var spans []ast.Span
	for index < len(lines) {
		content := bytes.TrimSpace(s.lineBytes(lines[index]))
		if len(content) >= len(marker) && bytes.Equal(content[:len(marker)], []byte(marker)) && allByte(content, marker[0]) {
			index++
			break
		}
		spans = append(spans, ast.Span{Start: lines[index].contentStart, End: lines[index].contentEnd})
		index++
	}
	end := lines[len(lines)-1].newlineEnd
	if index > 0 && index <= len(lines) {
		end = lines[index-1].newlineEnd
	}
	node := s.builder.Add(ast.CodeBlock, ast.Span{Start: lines[start].start, End: end})
	s.builder.SetText(node, joinSpans(s.source, spans, "\n")+func() string {
		if len(spans) > 0 {
			return "\n"
		}
		return ""
	}())
	s.builder.SetTitle(node, info)
	s.builder.SetIntegers(node, int(marker[0]), len(marker))
	s.builder.AppendChild(parent, node)
	return index, nil
}

type listMarker struct {
	ordered   bool
	start     int
	width     int
	delimiter byte
}

func (s *parseState) parseList(lines []sourceLine, start int, parent ast.NodeID, first listMarker) (int, error) {
	index := start
	list := s.builder.Add(ast.List, ast.Span{Start: lines[start].start, End: lines[start].newlineEnd})
	if first.ordered {
		s.builder.SetFlags(list, ast.ListOrdered)
	}
	s.builder.SetIntegers(list, first.start, int(first.delimiter))
	s.builder.AppendChild(parent, list)
	for index < len(lines) {
		marker, ok := parseListMarker(s.lineBytes(lines[index]))
		if !ok || marker.ordered != first.ordered || marker.delimiter != first.delimiter {
			break
		}
		itemStart := index
		adjusted := lines[index]
		adjusted.contentStart += marker.width
		itemLines := []sourceLine{adjusted}
		index++
		for index < len(lines) {
			candidate := s.lineBytes(lines[index])
			if next, matched := parseListMarker(candidate); matched && next.ordered == first.ordered && next.delimiter == first.delimiter {
				break
			}
			if !isBlank(candidate) && indentWidth(candidate) == 0 {
				break
			}
			continuation := lines[index]
			trim := byteOffsetForIndent(candidate, minInt(indentWidth(candidate), marker.width))
			continuation.contentStart += trim
			itemLines = append(itemLines, continuation)
			index++
		}
		item := s.builder.Add(ast.ListItem, ast.Span{Start: lines[itemStart].start, End: lines[index-1].newlineEnd})
		s.builder.AppendChild(list, item)
		if s.parser.profile.Has(profile.TaskLists) {
			firstContent := s.lineBytes(itemLines[0])
			if checked, consumed, ok := taskMarker(firstContent); ok {
				check := s.builder.Add(ast.TaskCheck, ast.Span{Start: itemLines[0].contentStart, End: itemLines[0].contentStart + consumed})
				if checked {
					s.builder.SetFlags(check, ast.TaskChecked)
				}
				s.builder.AppendChild(item, check)
				itemLines[0].contentStart += consumed
			}
		}
		if err := s.parseBlocks(itemLines, item); err != nil {
			return 0, err
		}
	}
	// The arena stores immutable spans, so the list span is represented by its
	// child item spans during rendering and source mapping.
	return index, nil
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
	return s.source[line.contentStart:line.contentEnd]
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

func atxHeading(line []byte) (level, start, end int, ok bool) {
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
	if closing < end && closing > index && (line[closing-1] == ' ' || line[closing-1] == '\t') {
		end = len(bytes.TrimRight(line[index:closing], " \t")) + index
	}
	return level, index, end, true
}

func setextUnderline(line []byte) (level int, marker byte, ok bool) {
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
	trimmed := bytes.TrimLeft(line, " \t")
	if len(line)-len(trimmed) > 3 || len(trimmed) < 3 || trimmed[0] != '`' && trimmed[0] != '~' {
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
	index := byteOffsetForIndent(content, 3)
	if index >= len(content) || content[index] != '>' {
		return sourceLine{}, false
	}
	index++
	if index < len(content) && (content[index] == ' ' || content[index] == '\t') {
		index++
	}
	line.contentStart += index
	return line, true
}

func parseListMarker(line []byte) (listMarker, bool) {
	indent := byteOffsetForIndent(line, 3)
	if indent >= len(line) {
		return listMarker{}, false
	}
	if line[indent] == '-' || line[indent] == '+' || line[indent] == '*' {
		if indent+1 >= len(line) || line[indent+1] != ' ' && line[indent+1] != '\t' {
			return listMarker{}, false
		}
		width := indent + 2
		for width < len(line) && width < indent+5 && line[width] == ' ' {
			width++
		}
		return listMarker{width: width, delimiter: line[indent]}, true
	}
	index := indent
	for index < len(line) && index-indent < 9 && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	if index == indent || index >= len(line) || line[index] != '.' && line[index] != ')' {
		return listMarker{}, false
	}
	if index+1 >= len(line) || line[index+1] != ' ' && line[index+1] != '\t' {
		return listMarker{}, false
	}
	start, err := strconv.Atoi(string(line[indent:index]))
	if err != nil {
		return listMarker{}, false
	}
	return listMarker{ordered: true, start: start, width: index + 2, delimiter: line[index]}, true
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
	if _, _, ok := fenceStart(line); ok || isThematicBreak(line) || looksLikeHTMLBlock(line) {
		return true
	}
	if _, ok := parseListMarker(line); ok {
		return true
	}
	trimmed := bytes.TrimLeft(line, " \t")
	return len(trimmed) > 0 && trimmed[0] == '>'
}

func looksLikeHTMLBlock(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) < 3 || trimmed[0] != '<' {
		return false
	}
	return bytes.HasPrefix(trimmed, []byte("<!--")) || bytes.HasPrefix(trimmed, []byte("<?")) || bytes.HasPrefix(trimmed, []byte("<![CDATA[")) || trimmed[1] == '/' || trimmed[1] == '!' || unicode.IsLetter(rune(trimmed[1]))
}

func referenceDefinition(line []byte) (destination, title, label string, ok bool) {
	trimmed := strings.TrimSpace(string(line))
	if !strings.HasPrefix(trimmed, "[") {
		return "", "", "", false
	}
	separator := strings.Index(trimmed, "]:")
	if separator <= 1 {
		return "", "", "", false
	}
	label = trimmed[1:separator]
	remainder := strings.TrimSpace(trimmed[separator+2:])
	if remainder == "" {
		return "", "", "", false
	}
	fields := strings.Fields(remainder)
	destination = strings.Trim(fields[0], "<>")
	if len(fields) > 1 {
		title = strings.Trim(strings.Join(fields[1:], " "), "\"'()")
	}
	return destination, title, label, true
}

func normalizeReference(label string) string {
	return strings.ToLower(strings.Join(strings.Fields(label), " "))
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

func joinSpans(source []byte, spans []ast.Span, separator string) string {
	var builder strings.Builder
	for index, span := range spans {
		if index > 0 {
			builder.WriteString(separator)
		}
		builder.Write(source[span.Start:span.End])
	}
	return builder.String()
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
