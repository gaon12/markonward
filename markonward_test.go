package markonward_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gaon12/markonward"
	"github.com/gaon12/markonward/profile"
	markhtml "github.com/gaon12/markonward/renderer/html"
)

func TestEngineComposesExplicitComponents(t *testing.T) {
	t.Parallel()
	engine, err := markonward.New(profile.EnhanceMarkV1, markhtml.New())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	result, err := engine.Convert(context.Background(), &output, []byte("**unfinished"))
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "<p><strong>unfinished</strong></p>\n" || len(result.Diagnostics) != 1 {
		t.Fatalf("output=%q diagnostics=%#v", output.String(), result.Diagnostics)
	}
}
