package trace

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"unicode/utf8"
)

// Locale selects a human trace catalog.
type Locale string

const (
	English Locale = "en"
	Korean  Locale = "ko"
)

func ParseLocale(value string) (Locale, error) {
	switch value {
	case "en", "english":
		return English, nil
	case "ko", "korean":
		return Korean, nil
	default:
		return "", fmt.Errorf("trace: unsupported locale %q", value)
	}
}

// Text writes localized one-line event descriptions.
type Text struct {
	mutex      sync.Mutex
	writer     io.Writer
	source     []byte
	locale     Locale
	lineStarts []int
}

func NewText(writer io.Writer, source []byte, locale Locale) (*Text, error) {
	if writer == nil {
		return nil, fmt.Errorf("trace: text writer is nil")
	}
	if locale != English && locale != Korean {
		return nil, fmt.Errorf("trace: unsupported locale %q", locale)
	}
	starts := []int{0}
	for index, current := range source {
		if current == '\n' {
			starts = append(starts, index+1)
		}
	}
	return &Text{writer: writer, source: source, locale: locale, lineStarts: starts}, nil
}

func (t *Text) Record(event Event) error {
	if t == nil || t.writer == nil {
		return fmt.Errorf("trace: text sink is nil")
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()
	line, column := t.position(event.Span.Start)
	_, err := fmt.Fprintf(t.writer, "[%d:%d @%d] %s\n", line, column, event.Span.Start, t.message(event))
	return err
}

func (t *Text) position(offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(t.source) {
		offset = len(t.source)
	}
	line := sort.Search(len(t.lineStarts), func(index int) bool { return t.lineStarts[index] > offset }) - 1
	if line < 0 {
		line = 0
	}
	return line + 1, utf8.RuneCount(t.source[t.lineStarts[line]:offset]) + 1
}

func (t *Text) message(event Event) string {
	if t.locale == Korean {
		return koreanMessage(event)
	}
	return englishMessage(event)
}

func englishMessage(event Event) string {
	switch event.RuleID {
	case "inline.delimiter.found":
		return fmt.Sprintf("found %s delimiter", field(event, "delimiter"))
	case "commonmark.inline.emphasis.flanking":
		return fmt.Sprintf("CommonMark emphasis flanking condition %s", event.Decision)
	case "enhance.inline.emphasis.paired-punctuation":
		return "applied EnhanceMark paired-punctuation emphasis rule"
	case "enhance.inline.tilde.range":
		return "kept a single tilde as a Korean range separator"
	case "enhance.inline.recovery.paragraph-end":
		return fmt.Sprintf("recovered unclosed %s at paragraph end", field(event, "kind"))
	case "inline.node.created":
		return fmt.Sprintf("created %s node", event.NodeKind)
	default:
		return fmt.Sprintf("%s: %s", event.RuleID, event.Decision)
	}
}

func koreanMessage(event Event) string {
	switch event.RuleID {
	case "inline.delimiter.found":
		return fmt.Sprintf("%q 구분자 발견", field(event, "delimiter"))
	case "commonmark.inline.emphasis.flanking":
		if event.Decision == Rejected {
			return "CommonMark 강조 구분자 조건 불충족"
		}
		return "CommonMark 강조 구분자 조건 충족"
	case "enhance.inline.emphasis.paired-punctuation":
		return "EnhanceMark 짝 구두점 인접 규칙 적용"
	case "enhance.inline.tilde.range":
		return "단일 물결표를 한국어 범위 구분자로 보존"
	case "enhance.inline.recovery.paragraph-end":
		return fmt.Sprintf("닫히지 않은 %s 서식을 문단 끝에서 복구", field(event, "kind"))
	case "inline.node.created":
		return fmt.Sprintf("%s 노드 생성", event.NodeKind)
	default:
		return fmt.Sprintf("%s: %s", event.RuleID, event.Decision)
	}
}

func field(event Event, name string) string {
	for _, current := range event.Fields {
		if current.Name == name {
			return current.Value
		}
	}
	return ""
}
