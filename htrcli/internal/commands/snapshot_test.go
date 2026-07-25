package commands

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

func TestPrintSnapshotResult(t *testing.T) {
	result := &api.CommandResult{
		Success: true,
		Data: map[string]any{
			"role": "document",
			"name": "Snapshot page",
			"children": []any{
				map[string]any{
					"role": "main",
					"children": []any{
						map[string]any{
							"role": "button",
							"name": "Save",
							"ref":  "@e1",
							"state": map[string]any{
								"pressed": true,
							},
						},
					},
				},
			},
		},
		PageInfo: &api.PageInfo{URL: "https://example.com", Title: "Example"},
	}

	output := captureStdout(t, func() {
		if err := printSnapshotResult(result); err != nil {
			t.Fatalf("printSnapshotResult returned error: %v", err)
		}
	})

	for _, want := range []string{
		"URL:   https://example.com",
		"Title: Example",
		"document \"Snapshot page\"",
		"  main",
		"    button \"Save\" @e1 [pressed]",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	outCh := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outCh <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = old
	return <-outCh
}
