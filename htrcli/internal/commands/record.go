package commands

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/media"
	"github.com/u007/htrcli/internal/output"
)

var (
	recordFindFFmpeg      = media.FindFFmpeg
	recordFFmpegVersion   = media.FFmpegVersion
	recordWriteRecording  = cdp.WriteRecording
	recordCleanupRecorder = stopRecorder
	recordRemoveAll       = os.RemoveAll
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record the page to video (Chrome/CDP only)",
}

// errRecordUnsupported is the explicit non-CDP guard. Video capture needs CDP
// Page.startScreencast, which the extension/Firefox transport cannot provide.
func errRecordUnsupported() error {
	return fmt.Errorf("record is not supported on Firefox or the extension transport — it requires Chrome via --cdp")
}

// recordFramesDir returns a fresh per-recording temp frames dir under ~/.htrcli.
func recordFramesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	dir := filepath.Join(home, ".htrcli", "recordings", fmt.Sprintf("rec-%d", time.Now().UnixMilli()), "frames")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creating frames dir: %w", err)
	}
	return dir, nil
}

// processAlive reports whether pid is a live process (signal 0 probe).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// recordStartLockPath returns the best-effort lock file used to serialize
// record start across processes.
func recordStartLockPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".htrcli", "recording.start.lock"), nil
}

// acquireRecordStartLock serializes `record start` across processes. A stale
// lock is removed when its recorded PID is no longer alive.
func acquireRecordStartLock() (func() error, error) {
	path, err := recordStartLockPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("creating recording lock dir: %w", err)
	}

	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			if cerr := f.Close(); cerr != nil {
				return nil, fmt.Errorf("closing recording lock: %w", cerr)
			}
			return func() error {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("removing recording lock: %w", err)
				}
				return nil
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("creating recording lock: %w", err)
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("reading recording lock: %w", readErr)
		}
		ownerPID, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		if ownerPID > 0 && processAlive(ownerPID) {
			return nil, fmt.Errorf("a recording start is already in progress")
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("removing stale recording lock: %w", err)
		}
	}
}

var recordStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start recording the current page",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !UseCDP() {
			return errRecordUnsupported()
		}

		releaseLock, err := acquireRecordStartLock()
		if err != nil {
			return err
		}
		defer func() {
			if err := releaseLock(); err != nil {
				fmt.Fprintf(os.Stderr, "[htrcli] releasing recording start lock: %v\n", err)
			}
		}()

		// Fail fast if ffmpeg is missing — never start a capture we can't encode.
		ffmpegPath, err := recordFindFFmpeg()
		if err != nil {
			return err
		}
		if ver, verr := recordFFmpegVersion(ffmpegPath); verr == nil {
			fmt.Fprintf(os.Stderr, "[htrcli] using %s\n", ver)
		}

		if st, err := cdp.ReadRecording(); err != nil {
			return err
		} else if st != nil {
			if st.PID > 0 && processAlive(st.PID) {
				return fmt.Errorf("a recording is already in progress (pid %d, frames %s) — stop it first", st.PID, st.FramesDir)
			}
			if err := cdp.RemoveRecording(); err != nil {
				return err
			}
			if st.FramesDir != "" {
				if err := recordRemoveAll(st.FramesDir); err != nil {
					fmt.Fprintf(os.Stderr, "[htrcli] cleaning stale frames dir %s: %v\n", st.FramesDir, err)
				}
			}
		}

		if err := ensureContextResolved(); err != nil {
			return err
		}

		framesDir, err := recordFramesDir()
		if err != nil {
			return err
		}
		port := GetCDPPort()

		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolving htrcli path: %w", err)
		}
		runArgs := []string{"record", "_run", "--frames-dir", framesDir, "--rec-port", strconv.Itoa(port)}
		if target := GetTabTarget(); target != "" {
			runArgs = append(runArgs, "--rec-tab", target)
		}
		child := exec.Command(exe, runArgs...)
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach: survives this CLI exiting
		if err := child.Start(); err != nil {
			if rmErr := recordRemoveAll(framesDir); rmErr != nil {
				fmt.Fprintf(os.Stderr, "[htrcli] cleaning frames dir %s after spawn failure: %v\n", framesDir, rmErr)
			}
			return fmt.Errorf("spawning recorder: %w", err)
		}

		st := &cdp.RecordingState{PID: child.Process.Pid, FramesDir: framesDir, Port: port, StartedAt: time.Now()}
		if err := recordWriteRecording(st); err != nil {
			if cleanupErr := recordCleanupRecorder(st); cleanupErr != nil {
				fmt.Fprintf(os.Stderr, "[htrcli] cleaning failed recorder start (pid %d): %v\n", st.PID, cleanupErr)
			}
			if rmErr := recordRemoveAll(framesDir); rmErr != nil {
				fmt.Fprintf(os.Stderr, "[htrcli] cleaning frames dir %s after write failure: %v\n", framesDir, rmErr)
			}
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(map[string]any{"recording": true, "pid": st.PID, "framesDir": framesDir})
			return nil
		}
		fmt.Printf("Recording started (pid %d). Stop with: htrcli record stop <output.mp4>\n", st.PID)
		return nil
	},
}

var (
	recRunFramesDir string
	recRunPort      int
	recRunTab       string
)

// recordRunCmd is the detached worker spawned by `record start`. Hidden — not a
// user-facing verb.
var recordRunCmd = &cobra.Command{
	Use:    "_run",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if recRunFramesDir == "" || recRunPort <= 0 {
			return fmt.Errorf("_run requires --frames-dir and --rec-port")
		}
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
		return cdp.RunRecorder(recRunPort, recRunTab, recRunFramesDir, stop)
	},
}

// stopRecorder signals the recorder to finish and waits for it to exit. It
// verifies the PID looks like our recorder (command line references the frames
// dir) before signalling, to avoid killing an unrelated reused PID. If the
// process does not exit within the grace period it is SIGKILLed.
func stopRecorder(st *cdp.RecordingState) error {
	if !processAlive(st.PID) {
		return nil // already gone — encode whatever frames exist
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(st.PID), "-o", "command=").Output()
	if err == nil && !strings.Contains(string(out), st.FramesDir) {
		fmt.Fprintf(os.Stderr, "[htrcli] pid %d does not look like the htrcli recorder (%s) — not signalling; encoding captured frames\n", st.PID, string(out))
		return nil
	}
	if err := syscall.Kill(st.PID, syscall.SIGTERM); err != nil {
		return fmt.Errorf("signalling recorder pid %d: %w", st.PID, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(st.PID) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "[htrcli] recorder pid %d did not exit in 10s — sending SIGKILL\n", st.PID)
	if err := syscall.Kill(st.PID, syscall.SIGKILL); err != nil {
		return fmt.Errorf("force-killing recorder pid %d: %w", st.PID, err)
	}
	return nil
}

// writeConcatList renders the ffmpeg concat list for frames into
// <framesDir>/concat.txt and returns its path.
func writeConcatList(framesDir string, frames []cdp.FrameMeta) (string, error) {
	// 30fps fallback for non-positive gaps / a lone final frame.
	list := cdp.BuildConcatList(frames, 1.0/30.0)
	path := filepath.Join(framesDir, "concat.txt")
	if err := os.WriteFile(path, []byte(list), 0644); err != nil {
		return "", fmt.Errorf("writing concat list: %w", err)
	}
	return path, nil
}

var recordStopCmd = &cobra.Command{
	Use:   "stop <output.mp4>",
	Short: "Stop recording and encode to MP4",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Preflight ffmpeg again — the environment may differ from record start.
		ffmpegPath, err := recordFindFFmpeg()
		if err != nil {
			return err
		}

		st, err := cdp.ReadRecording()
		if err != nil {
			return err
		}
		if st == nil {
			return fmt.Errorf("no recording in progress — start one with: htrcli record start")
		}

		if err := stopRecorder(st); err != nil {
			return err
		}

		frames, err := cdp.ReadFrameManifest(st.FramesDir)
		if err != nil {
			return fmt.Errorf("reading captured frames: %w", err)
		}
		if len(frames) == 0 {
			return fmt.Errorf("no frames captured — the page may have emitted no screencast frames")
		}

		concatPath, err := writeConcatList(st.FramesDir, frames)
		if err != nil {
			return err
		}

		outPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolving output path: %w", err)
		}
		if err := media.EncodeFrames(ffmpegPath, st.FramesDir, concatPath, outPath); err != nil {
			return err
		}

		// Success: clear state and reclaim the frames dir.
		if err := cdp.RemoveRecording(); err != nil {
			return err
		}
		if err := os.RemoveAll(st.FramesDir); err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] cleaning up frames dir %s: %v\n", st.FramesDir, err)
		}

		fmt.Printf("Recording saved to %s (%d frames)\n", outPath, len(frames))
		return nil
	},
}

func init() {
	recordRunCmd.Flags().StringVar(&recRunFramesDir, "frames-dir", "", "frames output directory (internal)")
	recordRunCmd.Flags().IntVar(&recRunPort, "rec-port", 0, "CDP debugging port (internal)")
	recordRunCmd.Flags().StringVar(&recRunTab, "rec-tab", "", "CDP target id (internal)")

	recordCmd.AddCommand(recordStartCmd)
	recordCmd.AddCommand(recordStopCmd)
	recordCmd.AddCommand(recordRunCmd)
	rootCmd.AddCommand(recordCmd)
}
