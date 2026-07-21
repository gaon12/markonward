package trace_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/trace"
)

func sampleEvent() trace.Event {
	return trace.Event{
		SchemaVersion: trace.SchemaVersion,
		Sequence:      1,
		Level:         trace.Decisions,
		Phase:         trace.Inline,
		RuleID:        "inline.delimiter.found",
		Decision:      trace.Observed,
		Span:          ast.Span{Start: 6, End: 8},
		Fields:        []trace.Field{{Name: "delimiter", Value: "**"}},
	}
}

func TestTextIsLocalizedAndRuneAware(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		locale trace.Locale
		want   string
	}{
		{name: "english", locale: trace.English, want: "[1:3 @6] found ** delimiter\n"},
		{name: "korean", locale: trace.Korean, want: "[1:3 @6] \"**\" 구분자 발견\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			sink, err := trace.NewText(&output, []byte("문장**"), test.locale)
			if err != nil {
				t.Fatal(err)
			}
			if err := sink.Record(sampleEvent()); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != test.want {
				t.Fatalf("text = %q, want %q", got, test.want)
			}
		})
	}
}

func TestJSONLinesUsesStableSchema(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	sink, err := trace.NewJSONLines(&output)
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Record(sampleEvent()); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(output.String(), "\n"); lines != 1 {
		t.Fatalf("JSON Lines newline count = %d", lines)
	}
	var decoded trace.Event
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != 1 || decoded.RuleID != "inline.delimiter.found" {
		t.Fatalf("decoded event = %#v", decoded)
	}
}
