// Command specsync downloads the immutable upstream conformance fixtures used
// by Markonward. Every payload is accepted only when its pinned SHA-256 matches.
package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type artifact struct {
	name   string
	url    string
	sha256 string
}

var artifacts = []artifact{
	{
		name:   "commonmark-0.31.2.json",
		url:    "https://spec.commonmark.org/0.31.2/spec.json",
		sha256: "d431b29d97b6f73e69d547109cf5081578fac931e72afe95639ebe766c1b2a20",
	},
	{
		name:   "gfm-0.29.0.gfm.0.txt",
		url:    "https://raw.githubusercontent.com/github/cmark-gfm/0.29.0.gfm.0/test/spec.txt",
		sha256: "7cea1221ffba48559d8748c8510d3c5bda40487a13667b80e77c14a1505b9821",
	},
}

func main() {
	output := flag.String("output", filepath.FromSlash("testdata/spec"), "fixture output directory")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := os.MkdirAll(*output, 0o750); err != nil {
		fatal(err)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	for _, item := range artifacts {
		if err := download(ctx, client, *output, item); err != nil {
			fatal(err)
		}
	}
}

func download(ctx context.Context, client *http.Client, output string, item artifact) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, item.url, nil)
	if err != nil {
		return fmt.Errorf("create request for %s: %w", item.name, err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download %s: %w", item.name, err)
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return fmt.Errorf("download %s: unexpected HTTP status %s", item.name, response.Status)
	}
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	closeErr := response.Body.Close()
	if readErr != nil {
		return fmt.Errorf("read %s: %w", item.name, readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close response for %s: %w", item.name, closeErr)
	}
	actual := fmt.Sprintf("%x", sha256.Sum256(payload))
	if actual != item.sha256 {
		return fmt.Errorf("verify %s: SHA-256 %s, want %s", item.name, actual, item.sha256)
	}
	target := filepath.Join(output, item.name)
	temporary := target + ".tmp"
	if err := os.WriteFile(temporary, payload, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", item.name, err)
	}
	if err := os.Rename(temporary, target); err != nil {
		return fmt.Errorf("install %s: %w", item.name, err)
	}
	fmt.Printf("verified %s (%d bytes)\n", target, len(payload))
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "specsync:", err)
	os.Exit(1)
}
