package hostcollate

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteSnapshotTightensExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX permission bits")
	}
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := SnapshotPath(cacheDir)
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSnapshot(cacheDir, Report{}, nil); err != nil {
		t.Fatal(err)
	}
	assertPermissions(t, cacheDir, 0o700)
	assertPermissions(t, path, 0o600)
}

func assertPermissions(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s permissions = %04o, want %04o", path, got, want)
	}
}
