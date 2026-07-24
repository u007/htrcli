package media

import (
	"errors"
	"strings"
	"testing"
)

func TestFindFFmpegMissing(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if _, err := FindFFmpeg(); err == nil {
		t.Fatal("expected error when ffmpeg is missing")
	}
}

func TestFindFFmpegPresent(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }
	path, err := FindFFmpeg()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/usr/local/bin/ffmpeg" {
		t.Fatalf("expected /usr/local/bin/ffmpeg, got %q", path)
	}
}

func TestEncodeArgs(t *testing.T) {
	args := EncodeArgs("/tmp/frames.txt", "/tmp/out.mp4")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-f concat", "-safe 0", "-i /tmp/frames.txt",
		"-vsync vfr", "-pix_fmt yuv420p", "/tmp/out.mp4",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected args to contain %q, got %q", want, joined)
		}
	}
}
