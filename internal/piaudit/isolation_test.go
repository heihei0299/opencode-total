package piaudit

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

const isolationChildEnv = "OPENCODE_ANALYZER_ISOLATION_CHILD"

func TestStandaloneModuleIsSelfContained(t *testing.T) {
	if os.Getenv(isolationChildEnv) == "1" {
		return
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate standalone module")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "../.."))
	copyRoot := filepath.Join(t.TempDir(), "opencode-analyzer")
	if err := copyDirectory(moduleRoot, copyRoot); err != nil {
		t.Fatalf("copy standalone module: %v", err)
	}

	cmd := exec.Command("go", "test", "-count=1", "-p", "1", "./...")
	cmd.Dir = copyRoot
	cmd.Env = append(os.Environ(), "GOMAXPROCS=2", isolationChildEnv+"=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("copied standalone module tests failed: %v\n%s", err, output)
	}
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(destination, 0o755)
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}
