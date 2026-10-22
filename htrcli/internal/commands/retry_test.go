package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestNextRetryDelay(t *testing.T) {
	cases := []struct {
		name    string
		current time.Duration
		max     time.Duration
		want    time.Duration
	}{
		{"doubles", 100 * time.Millisecond, 10 * time.Second, 200 * time.Millisecond},
		{"caps at max", 8 * time.Second, 10 * time.Second, 10 * time.Second},
		{"already at max", 10 * time.Second, 10 * time.Second, 10 * time.Second},
		{"never exceeds max even when double overflows", 6 * time.Second, 10 * time.Second, 10 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextRetryDelay(tc.current, tc.max); got != tc.want {
				t.Errorf("nextRetryDelay(%s, %s) = %s, want %s", tc.current, tc.max, got, tc.want)
			}
		})
	}
}

func TestRunWithRetrySucceedsFirstAttempt(t *testing.T) {
	calls := 0
	err := runWithRetry(3, time.Millisecond, time.Millisecond, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRunWithRetrySucceedsAfterFailures(t *testing.T) {
	calls := 0
	err := runWithRetry(3, time.Millisecond, time.Millisecond, func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRunWithRetryExhaustsAttempts(t *testing.T) {
	calls := 0
	last := errors.New("permanent")
	err := runWithRetry(2, time.Millisecond, time.Millisecond, func() error {
		calls++
		return last
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if calls != 3 { // times(2) + initial attempt
		t.Errorf("calls = %d, want 3", calls)
	}
	if !strings.Contains(err.Error(), "all 3 attempts failed") {
		t.Errorf("error %q should mention the attempt count", err)
	}
	if !errors.Is(err, last) {
		t.Errorf("error should wrap the last failure, got %v", err)
	}
}

func TestRunWithRetryZeroTimesRunsOnce(t *testing.T) {
	calls := 0
	err := runWithRetry(0, time.Millisecond, time.Millisecond, func() error {
		calls++
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRunWithRetryEmitsAttemptCount(t *testing.T) {
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	attempts := 0
	runErr := runWithRetry(2, time.Millisecond, time.Millisecond, func() error {
		attempts++
		if attempts < 2 {
			return fmt.Errorf("boom")
		}
		return nil
	})
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	os.Stderr = old
	out, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("runWithRetry: %v", runErr)
	}
	// The benchmark harness parses this exact line to report REAL attempts
	// rather than the retry budget it granted.
	if !strings.Contains(string(out), "retry: attempts=2") {
		t.Fatalf("stderr should carry the attempt count, got: %q", string(out))
	}
}

func TestInheritedGlobalArgsForwardsOnlyChangedFlags(t *testing.T) {
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	fs.String("transport", "", "")
	fs.Bool("json", false, "")
	fs.String("tab", "", "")
	if err := fs.Parse([]string{"--transport=cdp", "--json"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := inheritedGlobalArgs(fs)
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--transport=cdp") {
		t.Errorf("transport flag must be forwarded, got %v", got)
	}
	if !strings.Contains(joined, "--json=true") {
		t.Errorf("json flag must be forwarded, got %v", got)
	}
	if strings.Contains(joined, "--tab") {
		t.Errorf("unset flags must not be forwarded, got %v", got)
	}
}
