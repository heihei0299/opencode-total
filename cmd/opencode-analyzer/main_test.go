package main

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/heihei0299/opencode-analyzer/internal/opencode"
	"github.com/heihei0299/opencode-analyzer/internal/syncer"
)

func captureMainOutput(run func() error) (string, error) {
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = writer
	runErr := run()
	_ = writer.Close()
	os.Stdout = previous
	output, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		return string(output), readErr
	}
	return string(output), runErr
}

func TestRunSyncRejectsUnusedPiDirFlag(t *testing.T) {
	if err := runSync([]string{"--pi-dir", t.TempDir()}); err == nil {
		t.Fatal("sync --pi-dir must be rejected")
	}
}

func TestRunSyncReportsFailedStatus(t *testing.T) {
	runner := func(_ syncer.Options) (opencode.SyncResult, error) {
		return opencode.SyncResult{Status: opencode.SyncFailed, Reason: opencode.SyncReasonServer}, errors.New("failed")
	}
	output, err := captureMainOutput(func() error {
		return runSyncWithRunner([]string{"--data-dir", t.TempDir()}, runner)
	})
	if err == nil {
		t.Fatalf("failed sync must return an error; output=%s", output)
	}
	if !strings.Contains(output, "failed") {
		t.Fatalf("failed sync output = %s", output)
	}
}

func TestRunSyncReportsFailedProgress(t *testing.T) {
	lastSyncedTime := "2026-09-01T00:00:00Z"
	runner := func(_ syncer.Options) (opencode.SyncResult, error) {
		return opencode.SyncResult{
			Status:         opencode.SyncFailed,
			Reason:         opencode.SyncReasonHTTP,
			Pages:          2,
			LastSyncedTime: lastSyncedTime,
		}, errors.New("failed")
	}
	output, err := captureMainOutput(func() error {
		return runSyncWithRunner([]string{"--data-dir", t.TempDir()}, runner)
	})
	if err == nil {
		t.Fatalf("failed sync must return an error; output=%s", output)
	}
	for _, want := range []string{"同步失败", "2", lastSyncedTime} {
		if !strings.Contains(output, want) {
			t.Fatalf("failed sync output = %s, missing %q", output, want)
		}
	}
}

func TestRunSyncReportsPartialAndReturnsError(t *testing.T) {
	runner := func(_ syncer.Options) (opencode.SyncResult, error) {
		return opencode.SyncResult{
			Status:   opencode.SyncPartial,
			Warnings: []string{"月度成本未更新: temporary failure"},
		}, nil
	}
	output, err := captureMainOutput(func() error {
		return runSyncWithRunner([]string{"--data-dir", t.TempDir()}, runner)
	})
	if err == nil {
		t.Fatalf("partial sync must return a non-zero error; output=%s", output)
	}
	if !strings.Contains(output, "partial") || !strings.Contains(output, "月度成本未更新") {
		t.Fatalf("partial sync output = %s", output)
	}
}
