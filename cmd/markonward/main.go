package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
	"github.com/gaon12/markonward/renderer"
	markhtml "github.com/gaon12/markonward/renderer/html"
	markmarkdown "github.com/gaon12/markonward/renderer/markdown"
	"github.com/gaon12/markonward/renderer/plaintext"
	"github.com/gaon12/markonward/trace"
)

var version = "dev"

func main() {
	os.Exit(execute(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func execute(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		_, _ = fmt.Fprintln(stdout, version)
		return 0
	}
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	var err error
	switch args[0] {
	case "convert":
		err = runConvert(ctx, args[1:], stdin, stdout)
	case "explain":
		err = runExplain(ctx, args[1:], stdin, stdout)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		err = usageError("unknown command %q", args[0])
	}
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintln(stderr, "markonward:", err)
	var usage commandUsageError
	if errors.As(err, &usage) {
		printUsage(stderr)
		return 2
	}
	return 1
}

func runConvert(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("convert", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var from, target, output string
	var unsafeHTML bool
	flags.StringVar(&from, "from", "enhance", "input profile")
	flags.StringVar(&target, "to", "html", "output format")
	flags.StringVar(&output, "o", "", "output file")
	flags.StringVar(&output, "output", "", "output file")
	flags.BoolVar(&unsafeHTML, "unsafe-html", false, "allow trusted raw HTML")
	ordered := reorderArgs(args, map[string]bool{"--from": true, "--to": true, "-o": true, "--output": true})
	if err := flags.Parse(ordered); err != nil {
		return usageError("convert: %v", err)
	}
	if flags.NArg() > 1 {
		return usageError("convert accepts at most one input file")
	}
	if unsafeHTML && target != "html" {
		return usageError("--unsafe-html is valid only with --to html")
	}
	selected, err := profile.Parse(from)
	if err != nil {
		return usageError("convert: %v", err)
	}
	source, err := readInput(flags.Args(), stdin)
	if err != nil {
		return err
	}
	p, err := parser.New(selected)
	if err != nil {
		return err
	}
	result, err := p.Parse(ctx, source)
	if err != nil {
		return err
	}
	var outputRenderer renderer.Renderer
	switch target {
	case "html":
		if unsafeHTML {
			outputRenderer = markhtml.New(markhtml.WithUnsafe())
		} else {
			outputRenderer = markhtml.New()
		}
	case "text", "plaintext":
		outputRenderer = plaintext.New()
	case "markdown", "md":
		outputRenderer = markmarkdown.New(selected)
	default:
		return usageError("unknown output format %q", target)
	}
	writer, closeWriter, err := outputDestination(output, stdout)
	if err != nil {
		return err
	}
	if closeWriter != nil {
		defer closeWriter()
	}
	return outputRenderer.Render(ctx, writer, result.Document)
}

func runExplain(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("explain", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var selectedName, format, localeName, levelName, output string
	flags.StringVar(&selectedName, "profile", "enhance", "input profile")
	flags.StringVar(&format, "format", "text", "trace format")
	flags.StringVar(&localeName, "locale", "en", "text locale")
	flags.StringVar(&levelName, "level", "decisions", "trace level")
	flags.StringVar(&output, "o", "", "output file")
	flags.StringVar(&output, "output", "", "output file")
	ordered := reorderArgs(args, map[string]bool{"--profile": true, "--format": true, "--locale": true, "--level": true, "-o": true, "--output": true})
	if err := flags.Parse(ordered); err != nil {
		return usageError("explain: %v", err)
	}
	if flags.NArg() > 1 {
		return usageError("explain accepts at most one input file")
	}
	selected, err := profile.Parse(selectedName)
	if err != nil {
		return usageError("explain: %v", err)
	}
	level, err := trace.ParseLevel(levelName)
	if err != nil {
		return usageError("explain: %v", err)
	}
	source, err := readInput(flags.Args(), stdin)
	if err != nil {
		return err
	}
	writer, closeWriter, err := outputDestination(output, stdout)
	if err != nil {
		return err
	}
	if closeWriter != nil {
		defer closeWriter()
	}
	var sink trace.Sink
	switch format {
	case "text":
		locale, localeErr := trace.ParseLocale(localeName)
		if localeErr != nil {
			return usageError("explain: %v", localeErr)
		}
		sink, err = trace.NewText(writer, source, locale)
	case "jsonl", "json-lines":
		sink, err = trace.NewJSONLines(writer)
	default:
		return usageError("unknown trace format %q", format)
	}
	if err != nil {
		return err
	}
	p, err := parser.New(selected, parser.WithTrace(sink), parser.WithTraceLevel(level))
	if err != nil {
		return err
	}
	_, err = p.Parse(ctx, source)
	return err
}

func readInput(arguments []string, stdin io.Reader) ([]byte, error) {
	if len(arguments) == 0 || arguments[0] == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(arguments[0])
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", arguments[0], err)
	}
	return data, nil
}

func outputDestination(path string, stdout io.Writer) (io.Writer, func(), error) {
	if path == "" || path == "-" {
		return stdout, nil, nil
	}
	// #nosec G304 -- the CLI writes only to the path explicitly supplied by its user.
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create %s: %w", path, err)
	}
	return file, func() { _ = file.Close() }, nil
}

func reorderArgs(arguments []string, valueFlags map[string]bool) []string {
	flags := make([]string, 0, len(arguments))
	positionals := make([]string, 0, 1)
	for index := 0; index < len(arguments); index++ {
		current := arguments[index]
		if current == "--" {
			positionals = append(positionals, arguments[index+1:]...)
			break
		}
		if !strings.HasPrefix(current, "-") || current == "-" {
			positionals = append(positionals, current)
			continue
		}
		flags = append(flags, current)
		name := current
		if equals := strings.IndexByte(name, '='); equals >= 0 {
			name = name[:equals]
		}
		if valueFlags[name] && !strings.ContainsRune(current, '=') && index+1 < len(arguments) {
			index++
			flags = append(flags, arguments[index])
		}
	}
	return append(flags, positionals...)
}

type commandUsageError string

func (e commandUsageError) Error() string { return string(e) }

func usageError(format string, arguments ...any) error {
	return commandUsageError(fmt.Sprintf(format, arguments...))
}

func printUsage(writer io.Writer) {
	_, _ = fmt.Fprintln(writer, `Usage:
  markonward convert [FILE] [--from enhance|commonmark|gfm|gfm029] [--to html|text|markdown] [-o FILE] [--unsafe-html]
  markonward explain [FILE] [--profile enhance|commonmark|gfm|gfm029] [--format text|jsonl] [--locale en|ko] [--level decisions|verbose]
  markonward --version

FILE defaults to standard input. Output defaults to standard output.`)
}
