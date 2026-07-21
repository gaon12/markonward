// Package parser implements source-aware CommonMark, GFM, and EnhanceMark parsing.
package parser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/diagnostic"
	"github.com/gaon12/markonward/extension"
	"github.com/gaon12/markonward/profile"
	"github.com/gaon12/markonward/trace"
)

var (
	ErrInvalidUTF8   = errors.New("parser: input is not valid UTF-8")
	ErrInputTooLarge = errors.New("parser: input exceeds configured limit")
)

// Result contains a parsed document and non-fatal syntax diagnostics.
type Result struct {
	Document    *ast.Document
	Diagnostics []diagnostic.Diagnostic
}

// Parser is immutable and safe for concurrent use.
type Parser struct {
	profile      profile.Profile
	traceSink    trace.Sink
	traceLevel   trace.Level
	recovery     map[ast.Kind]RecoveryPolicy
	extensions   extension.Set
	maxInputSize int64
}

// New builds a parser for an explicitly selected profile.
func New(selected profile.Profile, options ...Option) (*Parser, error) {
	if !selected.Valid() {
		return nil, fmt.Errorf("parser: profile is required")
	}
	configuration := defaultConfig()
	if selected.Has(profile.ParagraphEndRecovery) {
		configuration.recovery[ast.Emphasis] = RecoverAtParagraphEnd
		configuration.recovery[ast.Strong] = RecoverAtParagraphEnd
		configuration.recovery[ast.Strikethrough] = RecoverAtParagraphEnd
	}
	for index, option := range options {
		if option == nil {
			return nil, fmt.Errorf("parser: option %d is nil", index)
		}
		if err := option(&configuration); err != nil {
			return nil, fmt.Errorf("parser: apply option %d: %w", index, err)
		}
	}
	registry := extension.NewRegistry()
	extensions, err := registry.Freeze(configuration.extensions...)
	if err != nil {
		return nil, err
	}
	recovery := make(map[ast.Kind]RecoveryPolicy, len(configuration.recovery))
	for kind, policy := range configuration.recovery {
		recovery[kind] = policy
	}
	return &Parser{
		profile:      selected,
		traceSink:    configuration.traceSink,
		traceLevel:   configuration.traceLevel,
		recovery:     recovery,
		extensions:   extensions,
		maxInputSize: configuration.maxInputSize,
	}, nil
}

// Profile returns the immutable dialect used by p.
func (p *Parser) Profile() profile.Profile { return p.profile }

// Parse borrows source for the returned document.
func (p *Parser) Parse(ctx context.Context, source []byte) (Result, error) {
	return p.parse(ctx, source, true)
}

// ParseCopy copies source before parsing so the result owns it.
func (p *Parser) ParseCopy(ctx context.Context, source []byte) (Result, error) {
	return p.parse(ctx, source, false)
}

// ParseReader reads at most the configured limit and returns an owned document.
func (p *Parser) ParseReader(ctx context.Context, reader io.Reader) (Result, error) {
	if reader == nil {
		return Result{}, fmt.Errorf("parser: reader is nil")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	limited := io.LimitReader(reader, p.maxInputSize+1)
	source, err := io.ReadAll(limited)
	if err != nil {
		return Result{}, fmt.Errorf("parser: read input: %w", err)
	}
	if int64(len(source)) > p.maxInputSize {
		return Result{}, ErrInputTooLarge
	}
	return p.parse(ctx, source, false)
}

func (p *Parser) parse(ctx context.Context, source []byte, borrow bool) (Result, error) {
	if p == nil {
		return Result{}, fmt.Errorf("parser: parser is nil")
	}
	if ctx == nil {
		return Result{}, fmt.Errorf("parser: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if int64(len(source)) > p.maxInputSize {
		return Result{}, ErrInputTooLarge
	}
	if !utf8.Valid(source) {
		return Result{}, ErrInvalidUTF8
	}
	builder := ast.NewBuilder(p.profile.ID(), source, borrow)
	state := parseState{
		parser:     p,
		ctx:        ctx,
		source:     builder.Document().Source(),
		builder:    builder,
		borrowed:   borrow,
		references: make(map[string]reference),
	}
	if err := state.parseBlocks(scanLines(source), state.builder.Document().Root()); err != nil {
		return Result{}, err
	}
	if err := state.parseInlines(); err != nil {
		return Result{}, err
	}
	for _, registration := range p.extensions.Registrations(extension.TransformPhase) {
		transformer := registration.Handler.(extension.ASTTransformer)
		if err := transformer.Transform(ctx, state.builder); err != nil {
			return Result{}, fmt.Errorf("parser: transform %s: %w", registration.ID, err)
		}
	}
	document, err := state.builder.Build()
	if err != nil {
		return Result{}, err
	}
	result := Result{Document: document, Diagnostics: append([]diagnostic.Diagnostic(nil), state.diagnostics...)}
	return result, nil
}
