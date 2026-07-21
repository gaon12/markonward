package parser

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

type parsedReference struct {
	destination string
	title       string
	label       string
	end         int
}

func (s *parseState) parseReferenceDefinition(lines []sourceLine, start int) (parsedReference, int, bool) {
	first := s.lineBytes(lines[start])
	if !referenceLabelMayStart(first) {
		return parsedReference{}, start, false
	}
	var input bytes.Buffer
	lineCount := 0
	for index := start; index < len(lines); index++ {
		content := s.lineBytes(lines[index])
		if index > start && isBlank(content) {
			break
		}
		input.Write(content)
		input.WriteByte('\n')
		lineCount++
	}
	parsed, ok := parseReferenceDefinitionText(input.Bytes())
	if !ok {
		return parsedReference{}, start, false
	}
	consumed := bytes.Count(input.Bytes()[:parsed.end], []byte{'\n'})
	if consumed == 0 {
		consumed = 1
	}
	if consumed > lineCount {
		return parsedReference{}, start, false
	}
	return parsed, start + consumed, true
}

func referenceLabelMayStart(line []byte) bool {
	if indentWidth(line) > 3 {
		return false
	}
	position := 0
	for position < len(line) && (line[position] == ' ' || line[position] == '\t') {
		position++
	}
	return position < len(line) && line[position] == '['
}

func parseReferenceDefinitionText(data []byte) (parsedReference, bool) { //nolint:gocyclo // The grammar is clearer as a single forward scanner.
	var result parsedReference
	lineEnd := bytes.IndexByte(data, '\n')
	if lineEnd < 0 {
		lineEnd = len(data)
	}
	if indentWidth(data[:lineEnd]) > 3 {
		return result, false
	}
	position := 0
	for position < lineEnd && (data[position] == ' ' || data[position] == '\t') {
		position++
	}
	if position >= len(data) || data[position] != '[' {
		return result, false
	}
	position++
	labelStart := position
	escaped := false
	for position < len(data) {
		current := data[position]
		if current == '\\' && !escaped {
			escaped = true
			position++
			continue
		}
		if current == '[' && !escaped {
			return result, false
		}
		if current == ']' && !escaped {
			break
		}
		escaped = false
		position++
	}
	if position >= len(data) || position+1 >= len(data) || data[position+1] != ':' {
		return result, false
	}
	label := string(data[labelStart:position])
	if utf8.RuneCountInString(label) > 999 || strings.TrimSpace(label) == "" {
		return result, false
	}
	position += 2
	position = skipReferenceWhitespace(data, position)
	if position >= len(data) {
		return result, false
	}
	destinationStart := position
	if data[position] == '<' {
		position++
		destinationStart = position
		for position < len(data) && data[position] != '>' {
			if data[position] == '\n' || data[position] == '<' {
				return result, false
			}
			if data[position] == '\\' && position+1 < len(data) {
				position += 2
				continue
			}
			position++
		}
		if position >= len(data) {
			return result, false
		}
		result.destination = string(data[destinationStart:position])
		position++
	} else {
		depth := 0
		for position < len(data) {
			current := data[position]
			if current == '\\' && position+1 < len(data) {
				position += 2
				continue
			}
			if isReferenceWhitespace(current) {
				break
			}
			if current == '(' {
				depth++
			} else if current == ')' {
				if depth == 0 {
					break
				}
				depth--
			}
			position++
		}
		if position == destinationStart || depth != 0 {
			return result, false
		}
		result.destination = string(data[destinationStart:position])
	}
	lineFinish := skipReferenceHorizontalSpace(data, position)
	if lineFinish >= len(data) {
		result.end = lineFinish
		return finishReference(result, label), true
	}
	fallbackEnd := 0
	switch {
	case data[lineFinish] == '\n':
		lineFinish++
		fallbackEnd = lineFinish
		titleStart := skipReferenceHorizontalSpace(data, lineFinish)
		if titleStart >= len(data) || !isReferenceTitleOpener(data[titleStart]) {
			result.end = lineFinish
			return finishReference(result, label), true
		}
		position = titleStart
	case isReferenceTitleOpener(data[lineFinish]) && lineFinish > position:
		position = lineFinish
	default:
		return parsedReference{}, false
	}
	title, titleEnd, titleOK := parseReferenceTitle(data, position)
	if !titleOK {
		if fallbackEnd > 0 {
			result.end = fallbackEnd
			return finishReference(result, label), true
		}
		return parsedReference{}, false
	}
	result.title = title
	result.end = titleEnd
	return finishReference(result, label), true
}

func parseReferenceTitle(data []byte, position int) (string, int, bool) {
	opener := data[position]
	closer := opener
	if opener == '(' {
		closer = ')'
	}
	position++
	titleStart := position
	for position < len(data) && data[position] != closer {
		if data[position] == '\\' && position+1 < len(data) {
			position += 2
			continue
		}
		position++
	}
	if position >= len(data) {
		return "", 0, false
	}
	title := string(data[titleStart:position])
	position++
	position = skipReferenceHorizontalSpace(data, position)
	if position < len(data) && data[position] != '\n' {
		return "", 0, false
	}
	if position < len(data) {
		position++
	}
	return title, position, true
}

func finishReference(result parsedReference, label string) parsedReference {
	result.destination = decodeLinkText(result.destination)
	result.title = decodeLinkText(result.title)
	result.label = label
	return result
}

func skipReferenceWhitespace(data []byte, position int) int {
	for position < len(data) && isReferenceWhitespace(data[position]) {
		position++
	}
	return position
}

func skipReferenceHorizontalSpace(data []byte, position int) int {
	for position < len(data) && (data[position] == ' ' || data[position] == '\t') {
		position++
	}
	return position
}

func isReferenceWhitespace(current byte) bool {
	return current == ' ' || current == '\t' || current == '\n' || current == '\r'
}

func isReferenceTitleOpener(current byte) bool {
	return current == '\'' || current == '"' || current == '('
}
