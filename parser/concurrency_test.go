package parser_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
	markhtml "github.com/gaon12/markonward/renderer/html"
)

func TestParserAndRendererAreConcurrentSafe(t *testing.T) {
	p, err := parser.New(profile.EnhanceMarkV1)
	if err != nil {
		t.Fatal(err)
	}
	renderer := markhtml.New()
	const workers = 32
	var group sync.WaitGroup
	errors := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			source := []byte(fmt.Sprintf("# 문서 %d\n\n서울~부산 **\"강조\"**", index))
			result, parseErr := p.Parse(context.Background(), source)
			if parseErr != nil {
				errors <- parseErr
				return
			}
			var output bytes.Buffer
			if renderErr := renderer.Render(context.Background(), &output, result.Document); renderErr != nil {
				errors <- renderErr
			}
		}(worker)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}
