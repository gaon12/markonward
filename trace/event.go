// Package trace records structured, explainable Markdown parsing decisions.
package trace

import (
	"fmt"
	"sync"

	"github.com/gaon12/markonward/ast"
)

const SchemaVersion = 1

// Level controls trace detail.
type Level uint8

const (
	Decisions Level = iota + 1
	Verbose
)

func ParseLevel(value string) (Level, error) {
	switch value {
	case "decisions", "decision":
		return Decisions, nil
	case "verbose":
		return Verbose, nil
	default:
		return 0, fmt.Errorf("trace: unknown level %q", value)
	}
}

// Phase identifies where an event occurred.
type Phase string

const (
	Block     Phase = "block"
	Inline    Phase = "inline"
	Transform Phase = "transform"
	Render    Phase = "render"
)

// Decision is the result of applying a parsing rule.
type Decision string

const (
	Observed  Decision = "observed"
	Accepted  Decision = "accepted"
	Rejected  Decision = "rejected"
	Literal   Decision = "literal"
	Recovered Decision = "recovered"
)

// Field is ordered structured event metadata.
type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Event is the stable language-neutral trace schema.
type Event struct {
	SchemaVersion int      `json:"schema_version"`
	Sequence      uint64   `json:"sequence"`
	Level         Level    `json:"level"`
	Phase         Phase    `json:"phase"`
	RuleID        string   `json:"rule_id"`
	Decision      Decision `json:"decision"`
	Span          ast.Span `json:"span"`
	Left          string   `json:"left,omitempty"`
	Right         string   `json:"right,omitempty"`
	NodeKind      ast.Kind `json:"node_kind,omitempty"`
	Fields        []Field  `json:"fields,omitempty"`
}

// Sink receives trace events. Returning an error aborts parsing.
type Sink interface {
	Record(Event) error
}

// Collector stores events safely for later inspection.
type Collector struct {
	mutex  sync.Mutex
	events []Event
}

func (c *Collector) Record(event Event) error {
	c.mutex.Lock()
	c.events = append(c.events, cloneEvent(event))
	c.mutex.Unlock()
	return nil
}

func (c *Collector) Events() []Event {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	result := make([]Event, len(c.events))
	for index, event := range c.events {
		result[index] = cloneEvent(event)
	}
	return result
}

func cloneEvent(event Event) Event {
	event.Fields = append([]Field(nil), event.Fields...)
	return event
}

// Filter drops events above MaxLevel before forwarding them.
type Filter struct {
	MaxLevel Level
	Next     Sink
}

func (f Filter) Record(event Event) error {
	if f.Next == nil || event.Level > f.MaxLevel {
		return nil
	}
	return f.Next.Record(event)
}
