package parser

import (
	"bytes"
	"strings"
)

type htmlBlockRule struct {
	endToken        string
	caseInsensitive bool
	untilBlank      bool
	untilRawTagEnd  bool
}

// htmlBlockStart classifies the seven CommonMark HTML block forms. Type 7 is
// deliberately excluded when interrupting is true because a complete generic
// tag cannot interrupt an existing paragraph (CommonMark 4.6).
func htmlBlockStart(line []byte, interrupting bool) (htmlBlockRule, bool) {
	if indentWidth(line) > 3 {
		return htmlBlockRule{}, false
	}
	content := bytes.TrimLeft(line, " \t")
	if len(content) < 2 || content[0] != '<' {
		return htmlBlockRule{}, false
	}
	lower := bytes.ToLower(content)
	for _, tag := range []string{"script", "pre", "style", "textarea"} {
		prefix := "<" + tag
		if bytes.HasPrefix(lower, []byte(prefix)) && htmlBlockTagBoundary(content[len(prefix):]) {
			return htmlBlockRule{caseInsensitive: true, untilRawTagEnd: true}, true
		}
	}
	if bytes.HasPrefix(content, []byte("<!--")) {
		return htmlBlockRule{endToken: "-->"}, true
	}
	if bytes.HasPrefix(content, []byte("<?")) {
		return htmlBlockRule{endToken: "?>"}, true
	}
	if bytes.HasPrefix(content, []byte("<![CDATA[")) {
		return htmlBlockRule{endToken: "]]>", caseInsensitive: false}, true
	}
	if len(content) >= 3 && bytes.HasPrefix(content, []byte("<!")) && content[2] >= 'A' && content[2] <= 'Z' {
		return htmlBlockRule{endToken: ">"}, true
	}
	if tag, _, ok := htmlTagName(content); ok && commonMarkBlockTags[tag] {
		return htmlBlockRule{untilBlank: true}, true
	}
	if interrupting {
		return htmlBlockRule{}, false
	}
	closing, tag, tagEnd, ok := htmlOpenOrClosingTagEnd(content, 0, len(content))
	if !ok || !onlyHTMLSpace(content[tagEnd:]) {
		return htmlBlockRule{}, false
	}
	if !closing && rawHTMLBlockTags[tag] {
		return htmlBlockRule{}, false
	}
	return htmlBlockRule{untilBlank: true}, true
}

func htmlBlockTagBoundary(rest []byte) bool {
	return len(rest) == 0 || rest[0] == '>' || isHTMLSpace(rest[0])
}

func htmlTagName(content []byte) (string, int, bool) {
	position := 1
	if position < len(content) && content[position] == '/' {
		position++
	}
	start := position
	if position >= len(content) || !isASCIIAlpha(content[position]) {
		return "", 0, false
	}
	position++
	for position < len(content) && isHTMLTagNameByte(content[position]) {
		position++
	}
	if !htmlBlockTagBoundaryOrClosing(content[position:]) {
		return "", 0, false
	}
	return strings.ToLower(string(content[start:position])), position, true
}

func htmlBlockTagBoundaryOrClosing(rest []byte) bool {
	return htmlBlockTagBoundary(rest) || len(rest) >= 2 && rest[0] == '/' && rest[1] == '>'
}

func htmlBlockEnded(line []byte, rule htmlBlockRule) bool {
	if rule.untilBlank || rule.endToken == "" {
		if !rule.untilRawTagEnd {
			return false
		}
		lower := bytes.ToLower(line)
		for _, tag := range []string{"script", "pre", "style", "textarea"} {
			if bytes.Contains(lower, []byte("</"+tag+">")) {
				return true
			}
		}
		return false
	}
	if rule.caseInsensitive {
		return bytes.Contains(bytes.ToLower(line), []byte(rule.endToken))
	}
	return bytes.Contains(line, []byte(rule.endToken))
}

// inlineHTMLEnd recognizes one complete CommonMark HTML tag, comment,
// processing instruction, declaration, or CDATA section and returns its
// half-open end offset.
func inlineHTMLEnd(data []byte, start, end int) (int, bool) {
	if start < 0 || start >= end || data[start] != '<' {
		return start, false
	}
	remaining := data[start:end]
	switch {
	case bytes.HasPrefix(remaining, []byte("<!--")):
		if len(remaining) >= 5 && remaining[4] == '>' {
			return start + 5, true
		}
		if len(remaining) >= 6 && remaining[4] == '-' && remaining[5] == '>' {
			return start + 6, true
		}
		closing := bytes.Index(remaining[4:], []byte("-->"))
		if closing < 0 {
			return start, false
		}
		return start + 4 + closing + 3, true
	case bytes.HasPrefix(remaining, []byte("<?")):
		return delimitedHTMLEnd(remaining, start, 2, "?>")
	case bytes.HasPrefix(remaining, []byte("<![CDATA[")):
		return delimitedHTMLEnd(remaining, start, 9, "]]>")
	case len(remaining) >= 3 && bytes.HasPrefix(remaining, []byte("<!")) && remaining[2] >= 'A' && remaining[2] <= 'Z':
		position := 3
		for position < len(remaining) && remaining[position] >= 'A' && remaining[position] <= 'Z' {
			position++
		}
		if position >= len(remaining) || !isHTMLSpace(remaining[position]) {
			return start, false
		}
		closing := bytes.IndexByte(remaining[position:], '>')
		if closing < 0 {
			return start, false
		}
		return start + position + closing + 1, true
	default:
		_, _, position, ok := htmlOpenOrClosingTagEnd(data, start, end)
		return position, ok
	}
}

func delimitedHTMLEnd(remaining []byte, start, contentStart int, delimiter string) (int, bool) {
	closing := bytes.Index(remaining[contentStart:], []byte(delimiter))
	if closing < 0 {
		return start, false
	}
	return start + contentStart + closing + len(delimiter), true
}

func htmlOpenOrClosingTagEnd(data []byte, start, end int) (closing bool, tag string, next int, ok bool) {
	if start >= end || data[start] != '<' {
		return false, "", start, false
	}
	position := start + 1
	if position < end && data[position] == '/' {
		closing = true
		position++
	}
	tagStart := position
	if position >= end || !isASCIIAlpha(data[position]) {
		return false, "", start, false
	}
	position++
	for position < end && isHTMLTagNameByte(data[position]) {
		position++
	}
	tag = strings.ToLower(string(data[tagStart:position]))
	if closing {
		position = skipHTMLSpace(data, position, end)
		if position < end && data[position] == '>' {
			return true, tag, position + 1, true
		}
		return false, "", start, false
	}
	for {
		beforeSpace := position
		position = skipHTMLSpace(data, position, end)
		if position >= end {
			return false, "", start, false
		}
		if data[position] == '>' {
			return false, tag, position + 1, true
		}
		if data[position] == '/' && position+1 < end && data[position+1] == '>' {
			return false, tag, position + 2, true
		}
		if position == beforeSpace || !isHTMLAttributeNameStart(data[position]) {
			return false, "", start, false
		}
		position++
		for position < end && isHTMLAttributeNameByte(data[position]) {
			position++
		}
		afterName := position
		equalPosition := skipHTMLSpace(data, position, end)
		if equalPosition >= end || data[equalPosition] != '=' {
			position = afterName
			continue
		}
		position = equalPosition + 1
		position = skipHTMLSpace(data, position, end)
		if position >= end {
			return false, "", start, false
		}
		switch data[position] {
		case '\'', '"':
			quote := data[position]
			position++
			for position < end && data[position] != quote {
				position++
			}
			if position >= end {
				return false, "", start, false
			}
			position++
		default:
			valueStart := position
			for position < end && !isHTMLSpace(data[position]) && !strings.ContainsRune("\"'=<>`", rune(data[position])) {
				position++
			}
			if position == valueStart {
				return false, "", start, false
			}
		}
	}
}

func skipHTMLSpace(data []byte, position, end int) int {
	for position < end && isHTMLSpace(data[position]) {
		position++
	}
	return position
}

func onlyHTMLSpace(data []byte) bool {
	return skipHTMLSpace(data, 0, len(data)) == len(data)
}

func isHTMLSpace(current byte) bool {
	return current == ' ' || current == '\t' || current == '\n' || current == '\r'
}

func isHTMLTagNameByte(current byte) bool {
	return isASCIIAlpha(current) || current >= '0' && current <= '9' || current == '-'
}

func isHTMLAttributeNameStart(current byte) bool {
	return isASCIIAlpha(current) || current == '_' || current == ':'
}

func isHTMLAttributeNameByte(current byte) bool {
	return isHTMLAttributeNameStart(current) || current >= '0' && current <= '9' || current == '.' || current == '-'
}
