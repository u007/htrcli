package host

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureSocketParentDir(t *testing.T) {
	home := t.TempDir()
	socketPath := filepath.Join(home, ".htrcli", "daemon.sock")

	if err := ensureSocketParentDir(socketPath); err != nil {
		t.Fatalf("ensureSocketParentDir: %v", err)
	}

	info, err := os.Stat(filepath.Dir(socketPath))
	if err != nil {
		t.Fatalf("stat socket dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("socket parent is not a directory: %v", info.Mode())
	}
}
