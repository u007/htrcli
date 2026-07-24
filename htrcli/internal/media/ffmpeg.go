// Package media wraps the external ffmpeg binary used to stitch screencast
// frames into an MP4. ffmpeg is a prerequisite, not vendored — every entry
// point surfaces its absence as an explicit error.
package media

import (
	"fmt"
	"os/exec"
	"strings"
)

// lookPath is a package var so tests can stub PATH resolution.
var lookPath = exec.LookPath

// FindFFmpeg returns the ffmpeg binary path or an explicit error. Callers MUST
// surface this error rather than proceeding — a missing binary must never turn
// into a hang at encode time.
func FindFFmpeg() (string, error) {
	path, err := lookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found on PATH — install ffmpeg (>= 6) and retry: %w", err)
	}
	return path, nil
}

// FFmpegVersion returns the first line of `ffmpeg -version`, for logging so the
// build/version in use is captured in record output.
func FFmpegVersion(ffmpegPath string) (string, error) {
	out, err := exec.Command(ffmpegPath, "-version").Output()
	if err != nil {
		return "", fmt.Errorf("running %s -version: %w", ffmpegPath, err)
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return line, nil
}

// EncodeArgs returns the ffmpeg arguments for a concat-demuxer encode.
func EncodeArgs(concatListPath, output string) []string {
	return []string{
		"-f", "concat",
		"-safe", "0",
		"-i", concatListPath,
		"-vsync", "vfr",
		"-pix_fmt", "yuv420p",
		"-y",
		output,
	}
}

// EncodeFrames runs ffmpeg to stitch the frame list into the output MP4.
func EncodeFrames(ffmpegPath, workDir, concatListPath, output string) error {
	cmd := exec.Command(ffmpegPath, EncodeArgs(concatListPath, output)...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg encode failed: %w\noutput: %s", err, string(out))
	}
	return nil
}
