package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// selfPath returns the path of the running htrcli binary.
//
// Every self-re-invocation goes through this helper (the `retry` wrapper, the
// benchmark harness and the background recorder process), so there is exactly
// ONE place that knows how htrcli re-executes itself. Callers must not call
// os.Executable/exec.Command directly.
func selfPath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	return p, nil
}

// selfRun holds the result of one self-re-invocation.
type selfRun struct {
	// Stdout is the child's captured standard output.
	Stdout string
	// Stderr is the child's captured standard error.
	Stderr string
	// ExitCode is the child's exit code (0 on success, 1 when the child
	// could not be started).
	ExitCode int
	// Duration is the child's wall-clock runtime.
	Duration time.Duration
	// Err is the start/exec error, if any.
	Err error
}

// selfCommand builds an exec.Cmd that re-invokes htrcli with args.
func selfCommand(args []string) (*exec.Cmd, error) {
	self, err := selfPath()
	if err != nil {
		return nil, err
	}
	return exec.Command(self, args...), nil
}

// runSelf re-invokes the htrcli binary with args and captures its output.
//
// The child's stdout and stderr are captured (not forwarded) so a caller can
// assert on them. The child gets no TTY: the harness must not hang waiting
// for a human.
func runSelf(args []string) selfRun {
	start := time.Now()
	cmd, err := selfCommand(args)
	if err != nil {
		return selfRun{ExitCode: 1, Duration: time.Since(start), Err: err}
	}
	// Stderr is set explicitly so cmd.Output does not capture it (Output
	// would otherwise only expose it through the ExitError).
	var stderrBuf []byte
	cmd.Stderr = &byteSliceWriter{buf: &stderrBuf}
	stdout, err := cmd.Output()
	run := selfRun{
		Stdout:   string(stdout),
		Stderr:   string(stderrBuf),
		ExitCode: 0,
		Duration: time.Since(start),
	}
	if err != nil {
		run.Err = err
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			run.ExitCode = exitErr.ExitCode()
		} else {
			run.ExitCode = 1
		}
	}
	return run
}

// runSelfStream re-invokes the htrcli binary with args, forwarding its
// output to stdout/stderr so the user sees it live.
//
// It returns the child's exit code and its wall-clock duration.
func runSelfStream(args []string, stdin io.Reader) (int, time.Duration, error) {
	start := time.Now()
	cmd, err := selfCommand(args)
	if err != nil {
		return 1, time.Since(start), err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = stdin
	err = cmd.Run()
	dur := time.Since(start)
	if err == nil {
		return 0, dur, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), dur, err
	}
	return 1, dur, err
}

// byteSliceWriter appends to a byte slice pointer.
type byteSliceWriter struct{ buf *[]byte }

func (w *byteSliceWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}
