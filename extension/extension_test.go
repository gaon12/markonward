package extension_test

import (
	"testing"

	"github.com/gaon12/markonward/extension"
)

type inlineParser struct{}

func (inlineParser) ParseInline(extension.ParseContext) (extension.Match, bool, error) {
	return extension.Match{}, false, nil
}

func TestRegistryRejectsAmbiguousOrdering(t *testing.T) {
	t.Parallel()
	registry := extension.NewRegistry()
	if err := registry.Register(extension.Registration{
		ID: "mention", Phase: extension.InlinePhase, Priority: 100, Triggers: []byte{'@'}, Handler: inlineParser{},
	}); err != nil {
		t.Fatal(err)
	}
	err := registry.Register(extension.Registration{
		ID: "handle", Phase: extension.InlinePhase, Priority: 100, Triggers: []byte{'@'}, Handler: inlineParser{},
	})
	if err == nil {
		t.Fatal("overlapping trigger at an equal priority should fail")
	}
}

func TestSetIsOrderedAndDefensive(t *testing.T) {
	t.Parallel()
	registry := extension.NewRegistry()
	for _, registration := range []extension.Registration{
		{ID: "late", Phase: extension.InlinePhase, Priority: 200, Triggers: []byte{'!'}, Handler: inlineParser{}},
		{ID: "early", Phase: extension.InlinePhase, Priority: 10, Triggers: []byte{'?'}, Handler: inlineParser{}},
	} {
		if err := registry.Register(registration); err != nil {
			t.Fatal(err)
		}
	}
	set, err := registry.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	entries := set.Registrations(extension.InlinePhase)
	if len(entries) != 2 || entries[0].ID != "early" || entries[1].ID != "late" {
		t.Fatalf("unexpected order: %#v", entries)
	}
	entries[0].Triggers[0] = '#'
	if got := set.Registrations(extension.InlinePhase)[0].Triggers[0]; got != '?' {
		t.Fatalf("Set exposed mutable trigger storage: %q", got)
	}
}
