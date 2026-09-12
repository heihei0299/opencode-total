package syncer

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/heihei0299/opencode-analyzer/internal/opencode"
)

type syncerRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn syncerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func syncerRPCResponse(value any) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(opencode.EncodePayload([]any{value}))),
		Header:     make(http.Header),
	}
}

func TestRunAutomaticallySelectsSingleWorkspace(t *testing.T) {
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "")
	t.Setenv("OPENCODE_WORKSPACE", "")
	previousTransport := http.DefaultTransport
	http.DefaultTransport = syncerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.Header.Get("X-Server-Id") {
		case opencode.FnWorkspaces:
			return syncerRPCResponse(map[string]any{
				"workspaces": []any{map[string]any{"id": "wrk_one", "name": "One"}},
			}), nil
		case opencode.FnMonthlyCosts:
			return syncerRPCResponse(map[string]any{"usage": []any{}, "keys": []any{}}), nil
		case opencode.FnUsageHistory:
			return syncerRPCResponse([]opencode.UsageRecord{}), nil
		default:
			return nil, nil
		}
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	if _, err := Run(Options{DataDir: t.TempDir()}); err != nil {
		t.Fatalf("single workspace should be selected automatically: %v", err)
	}
}

func TestRunFailsWhenNoWorkspaceAvailable(t *testing.T) {
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "")
	t.Setenv("OPENCODE_WORKSPACE", "")
	previousTransport := http.DefaultTransport
	http.DefaultTransport = syncerRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return syncerRPCResponse(map[string]any{"workspaces": []any{}}), nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	if _, err := Run(Options{DataDir: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "无法自动发现工作区") {
		t.Fatalf("missing workspace must fail explicitly, got %v", err)
	}
}

func TestRunReturnsConflictWhenDataDirectoryIsLocked(t *testing.T) {
	dataDir := t.TempDir()
	storage := opencode.NewStorage(dataDir)
	unlock, err := storage.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "wrk_test")

	if _, err := Run(Options{DataDir: dataDir}); err == nil || !strings.Contains(err.Error(), "同步进行中") {
		t.Fatalf("locked sync must return a conflict, got %v", err)
	}
}

func TestRunRequiresExplicitWorkspaceWhenMultiple(t *testing.T) {
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "")
	t.Setenv("OPENCODE_WORKSPACE", "")
	previousTransport := http.DefaultTransport
	http.DefaultTransport = syncerRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return syncerRPCResponse(map[string]any{
			"workspaces": []any{
				map[string]any{"id": "wrk_one", "name": "One"},
				map[string]any{"id": "wrk_two", "name": "Two"},
			},
		}), nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	if _, err := Run(Options{DataDir: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "多个") {
		t.Fatalf("multiple workspaces must require an explicit selection, got %v", err)
	}
}
