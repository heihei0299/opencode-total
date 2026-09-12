package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPriorityAndDotEnv(t *testing.T) {
	isolateWorkingDirectory(t)
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, ".env"), []byte("OPENCODE_AUTH=file-auth\nOPENCODE_WORKSPACE_ID=file-workspace\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCODE_AUTH", "env-auth")
	t.Setenv("OPENCODE_WORKSPACE_ID", "")
	t.Setenv("OPENCODE_WORKSPACE", "")
	credentials, err := Load(Input{Auth: "cli-auth", DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Auth != "cli-auth" || credentials.Workspace != "file-workspace" || credentials.DataDir != dataDir {
		t.Fatalf("credentials = %+v", credentials)
	}

	credentials, err = Load(Input{DataDir: dataDir})
	if err != nil || credentials.Auth != "env-auth" {
		t.Fatalf("credentials = %+v, err = %v", credentials, err)
	}
}

func TestLoadRequiresAuth(t *testing.T) {
	isolateWorkingDirectory(t)
	t.Setenv("OPENCODE_AUTH", "")
	if _, err := Load(Input{DataDir: t.TempDir()}); err == nil {
		t.Fatal("expected missing auth error")
	}
}

func isolateWorkingDirectory(t *testing.T) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
}
