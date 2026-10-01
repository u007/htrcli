package commands

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

// This file binds the CLI's ZIP writer to shared/recording-export-contract.json.
//
// The extension's writer (src/utils/exportZip.ts) is bound to the same file by
// src/utils/exportZip.contract.test.ts. Together the two make it impossible for
// either producer to change the bundle layout without the contract — and
// therefore the other producer — being updated too.

// zipEntryMethods reports each entry's compression method.
func zipEntryMethods(t *testing.T, path string) map[string]uint16 {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("opening zip: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	out := map[string]uint16{}
	for _, f := range r.File {
		out[f.Name] = f.Method
	}
	return out
}

// contractExpectedEntries derives the entry names a session must produce,
// reading the media presence off the session so a step without audio is not
// expected to yield a .webm.
func contractExpectedEntries(t *testing.T, c *exportContract, s *api.RecordingSession) []string {
	t.Helper()
	names := map[string]bool{}
	for _, f := range c.BundleFiles {
		names[f] = true
	}
	for i := range s.Steps {
		n := i + 1
		if s.Steps[i].ScreenshotData != nil {
			p, err := c.contractPath("stepScreenshot", n)
			if err != nil {
				t.Fatalf("contract stepScreenshot: %v", err)
			}
			names[p] = true
		}
		if s.Steps[i].AudioData != nil {
			p, err := c.contractPath("stepAudio", n)
			if err != nil {
				t.Fatalf("contract stepAudio: %v", err)
			}
			names[p] = true
		}
	}
	for i := range s.Annotations {
		n := i + 1
		if s.Annotations[i].ScreenshotData != nil {
			p, err := c.contractPath("annotationScreenshot", n)
			if err != nil {
				t.Fatalf("contract annotationScreenshot: %v", err)
			}
			names[p] = true
		}
		if s.Annotations[i].AudioData != nil {
			p, err := c.contractPath("annotationAudio", n)
			if err != nil {
				t.Fatalf("contract annotationAudio: %v", err)
			}
			names[p] = true
		}
	}
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func TestExportBundleMatchesSharedContract(t *testing.T) {
	c, err := loadExportContract()
	if err != nil {
		t.Fatalf("loading shared contract: %v", err)
	}
	session := exportSession()

	out := filepath.Join(t.TempDir(), "contract.zip")
	if err := writeRecordingZip(out, session); err != nil {
		t.Fatalf("writeRecordingZip: %v", err)
	}

	entries := readZip(t, out)
	got := keysOf(entries)
	sort.Strings(got)
	want := contractExpectedEntries(t, c, session)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("bundle entry names diverged from the shared contract\n got: %v\nwant: %v", got, want)
	}
}

func TestExportBundleUsesContractCompression(t *testing.T) {
	c, err := loadExportContract()
	if err != nil {
		t.Fatalf("loading shared contract: %v", err)
	}
	switch c.Compression {
	case "deflate":
		// JSZip uses DEFLATE. A Store-only archive still unzips everywhere, so
		// this assertion is the only thing that catches the divergence.
	case "store":
		t.Log("contract now specifies store; update the expectation below")
	default:
		t.Fatalf("unknown compression %q in the shared contract", c.Compression)
	}

	out := filepath.Join(t.TempDir(), "compression.zip")
	if err := writeRecordingZip(out, exportSession()); err != nil {
		t.Fatalf("writeRecordingZip: %v", err)
	}
	methods := zipEntryMethods(t, out)
	if len(methods) == 0 {
		t.Fatal("expected entries in the archive")
	}
	for name, method := range methods {
		if c.Compression == "deflate" && method != zip.Deflate {
			t.Errorf("entry %q uses method %d, want deflate (%d) per the shared contract",
				name, method, zip.Deflate)
		}
	}
}

func TestExportManifestPathsMatchSharedContract(t *testing.T) {
	c, err := loadExportContract()
	if err != nil {
		t.Fatalf("loading shared contract: %v", err)
	}
	session := exportSession()
	exported := buildExportedRecording(session)

	// The manifest must reference the contract's paths verbatim.
	for i := range session.Steps {
		n := i + 1
		if session.Steps[i].ScreenshotData != nil {
			want, err := c.contractPath("stepScreenshot", n)
			if err != nil {
				t.Fatalf("contract stepScreenshot: %v", err)
			}
			if exported.Steps[i].ScreenshotPath != want {
				t.Errorf("step %d manifest path = %q, want %q (contract)", n, exported.Steps[i].ScreenshotPath, want)
			}
		}
		if session.Steps[i].AudioData != nil {
			want, err := c.contractPath("stepAudio", n)
			if err != nil {
				t.Fatalf("contract stepAudio: %v", err)
			}
			if exported.Steps[i].AudioPath != want {
				t.Errorf("step %d manifest audio path = %q, want %q (contract)", n, exported.Steps[i].AudioPath, want)
			}
		}
	}
	for i := range session.Annotations {
		n := i + 1
		if session.Annotations[i].ScreenshotData != nil {
			want, err := c.contractPath("annotationScreenshot", n)
			if err != nil {
				t.Fatalf("contract annotationScreenshot: %v", err)
			}
			if exported.Annotations[i].ScreenshotPath != want {
				t.Errorf("annotation %d manifest path = %q, want %q (contract)", n, exported.Annotations[i].ScreenshotPath, want)
			}
		}
	}

	// The manifest and README must use the contract's file names.
	raw, err := json.Marshal(exported)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "base64") {
		t.Error("recording.json must reference media by path, never inline base64")
	}
	entries := readZip(t, writeContractZip(t))
	for _, name := range c.BundleFiles {
		if _, ok := entries[name]; !ok {
			t.Errorf("bundle is missing the contract file %q (have %v)", name, keysOf(entries))
		}
	}
}

// writeContractZip produces a throwaway bundle and returns its path.
func writeContractZip(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "manifest.zip")
	if err := writeRecordingZip(out, exportSession()); err != nil {
		t.Fatalf("writeRecordingZip: %v", err)
	}
	return out
}

func TestExportContractIsSelfConsistent(t *testing.T) {
	c, err := loadExportContract()
	if err != nil {
		t.Fatalf("loading shared contract: %v", err)
	}
	// Every pattern must agree with its full path, or the two halves of the
	// contract could be edited inconsistently and both tests would still pass.
	dirs := map[string]bool{}
	for _, d := range c.Directories {
		dirs[d] = true
	}
	for _, key := range []string{"stepScreenshot", "stepAudio", "annotationScreenshot", "annotationAudio"} {
		pattern, ok := c.Patterns[key]
		if !ok {
			t.Errorf("contract is missing patterns.%s", key)
			continue
		}
		path, ok := c.Paths[key]
		if !ok {
			t.Errorf("contract is missing paths.%s", key)
			continue
		}
		dir, file, found := strings.Cut(path, "/")
		if !found {
			t.Errorf("paths.%s = %q has no directory component", key, path)
			continue
		}
		if !dirs[dir] {
			t.Errorf("paths.%s uses directory %q, which is not in directories %v", key, dir, c.Directories)
		}
		if file != pattern {
			t.Errorf("paths.%s = %q disagrees with patterns.%s = %q", key, path, key, pattern)
		}
	}
	// The contract must be reachable from the package dir, or every test using
	// it would silently skip.
	if _, err := os.Stat(filepath.Join("..", "..", "..", "shared")); err != nil {
		t.Logf("note: shared/ is not at the expected relative path: %v", err)
	}
}
