package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	retryTimes    int
	retryDelayMs  int
	retryMaxDelay int
)

var retryCmd = &cobra.Command{
	Use:   "retry -- <command> [args...]",
	Short: "Run a command with exponential-backoff retries on failure",
	Long: `retry runs an htrcli subcommand (everything after --) and, on a
non-zero exit, re-runs it with exponential backoff. Each attempt and its
outcome are printed to stderr so failures are never silent.

Exit code is non-zero when every attempt fails.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if retryTimes < 0 {
			return fmt.Errorf("--times must be >= 0")
		}
		if retryDelayMs <= 0 {
			return fmt.Errorf("--delay must be > 0 (ms)")
		}
		if retryMaxDelay < retryDelayMs {
			return fmt.Errorf("--max-delay must be >= --delay")
		}

		// A flag the user put BEFORE the subcommand (`htrcli --transport cdp
		// retry -- ...`) is consumed by this process's own flag parsing and is
		// therefore absent from `args`. Forward the ones that were actually
		// set, or the child silently runs with different settings — a
		// `--transport cdp` retry would otherwise run over the extension
		// transport.
		childArgs := append(inheritedGlobalArgs(rootCmd.PersistentFlags()), args...)

		return runWithRetry(
			retryTimes,
			time.Duration(retryDelayMs)*time.Millisecond,
			time.Duration(retryMaxDelay)*time.Millisecond,
			func() error {
				// The child writes to the real stdout/stderr so the user
				// sees the command's output live.
				cmd, err := selfCommand(childArgs)
				if err != nil {
					return err
				}
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Stdin = os.Stdin
				return cmd.Run()
			},
		)
	},
}

// runWithRetry executes run and, on error, retries it with exponential backoff.
// times is the number of RETRIES (total attempts = times+1). Each failed
// attempt and the delay before the next one are written to stderr so callers
// can see exactly what happened; the final error wraps the last failure.
func runWithRetry(times int, initialDelay, maxDelay time.Duration, run func() error) error {
	totalAttempts := times + 1
	delay := initialDelay
	var lastErr error
	for attempt := 1; attempt <= totalAttempts; attempt++ {
		err := run()
		if err == nil {
			// Machine-readable attempt count: callers that wrap this command
			// (the benchmark harness) need the attempts ACTUALLY made, not the
			// budget they granted. Keep the wording stable.
			fmt.Fprintf(os.Stderr, "retry: attempts=%d\n", attempt)
			if attempt > 1 {
				fmt.Fprintf(os.Stderr, "retry: succeeded on attempt %d/%d\n", attempt, totalAttempts)
			}
			return nil
		}
		lastErr = err
		if attempt < totalAttempts {
			fmt.Fprintf(os.Stderr, "retry: attempt %d/%d failed: %v — retrying in %s\n",
				attempt, totalAttempts, err, delay)
			time.Sleep(delay)
			delay = nextRetryDelay(delay, maxDelay)
		} else {
			fmt.Fprintf(os.Stderr, "retry: attempts=%d\n", totalAttempts)
			fmt.Fprintf(os.Stderr, "retry: all %d attempts failed\n", totalAttempts)
		}
	}
	return fmt.Errorf("retry: all %d attempts failed: %w", totalAttempts, lastErr)
}

// nextRetryDelay doubles the current delay, capping it at max.
func nextRetryDelay(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max || next <= 0 {
		return max
	}
	return next
}

func init() {
	retryCmd.Flags().IntVar(&retryTimes, "times", 3, "number of retries after the first attempt")
	retryCmd.Flags().IntVar(&retryDelayMs, "delay", 500, "initial backoff delay in ms")
	retryCmd.Flags().IntVar(&retryMaxDelay, "max-delay", 10000, "maximum backoff delay in ms")
	rootCmd.AddCommand(retryCmd)
}

// inheritedGlobalArgs renders the persistent flags the user actually set as
// CLI arguments, so a self-re-invoked child inherits them.
func inheritedGlobalArgs(fs *pflag.FlagSet) []string {
	var out []string
	fs.Visit(func(f *pflag.Flag) {
		out = append(out, "--"+f.Name+"="+f.Value.String())
	})
	return out
}
