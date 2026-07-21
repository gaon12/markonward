package parser

import (
	"fmt"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/extension"
	"github.com/gaon12/markonward/trace"
)

// RecoveryPolicy controls an incomplete inline construct.
type RecoveryPolicy uint8

const (
	Literal RecoveryPolicy = iota
	RecoverAtParagraphEnd
	Error
)

// Option configures an immutable Parser.
type Option func(*config) error

type config struct {
	traceSink    trace.Sink
	traceLevel   trace.Level
	recovery     map[ast.Kind]RecoveryPolicy
	extensions   []extension.Extension
	maxInputSize int64
}

func defaultConfig() config {
	return config{
		traceLevel: trace.Decisions,
		recovery: map[ast.Kind]RecoveryPolicy{
			ast.Emphasis:      Literal,
			ast.Strong:        Literal,
			ast.Strikethrough: Literal,
			ast.CodeSpan:      Literal,
			ast.Link:          Literal,
			ast.Image:         Literal,
		},
		maxInputSize: 64 << 20,
	}
}

// WithTrace enables structured parsing events.
func WithTrace(sink trace.Sink) Option {
	return func(configuration *config) error {
		if sink == nil {
			return fmt.Errorf("parser: trace sink is nil")
		}
		configuration.traceSink = sink
		return nil
	}
}

// WithTraceLevel selects the maximum emitted event detail.
func WithTraceLevel(level trace.Level) Option {
	return func(configuration *config) error {
		if level != trace.Decisions && level != trace.Verbose {
			return fmt.Errorf("parser: invalid trace level %d", level)
		}
		configuration.traceLevel = level
		return nil
	}
}

// WithRecovery overrides the incomplete construct policy for kind.
func WithRecovery(kind ast.Kind, policy RecoveryPolicy) Option {
	return func(configuration *config) error {
		if _, supported := configuration.recovery[kind]; !supported {
			return fmt.Errorf("parser: recovery is unsupported for %s", kind)
		}
		if policy > Error {
			return fmt.Errorf("parser: invalid recovery policy %d", policy)
		}
		if policy == RecoverAtParagraphEnd && (kind == ast.Link || kind == ast.Image) {
			return fmt.Errorf("parser: %s cannot be recovered at paragraph end", kind)
		}
		configuration.recovery[kind] = policy
		return nil
	}
}

// WithExtensions installs parser and AST transform extensions.
func WithExtensions(extensions ...extension.Extension) Option {
	return func(configuration *config) error {
		for index, current := range extensions {
			if current == nil {
				return fmt.Errorf("parser: extension %d is nil", index)
			}
		}
		configuration.extensions = append(configuration.extensions, extensions...)
		return nil
	}
}

// WithMaxInputBytes sets the ParseReader memory guard.
func WithMaxInputBytes(maximum int64) Option {
	return func(configuration *config) error {
		if maximum <= 0 {
			return fmt.Errorf("parser: maximum input size must be positive")
		}
		configuration.maxInputSize = maximum
		return nil
	}
}
