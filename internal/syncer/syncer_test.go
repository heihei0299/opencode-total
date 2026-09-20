package syncer

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestRunReportsPartialWhenMonthlyCostsFailAfterHistory(t *testing.T) {
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "wrk_test")
	t.Setenv("OPENCODE_WORKSPACE", "")
	var calls []string
	usageCalls := 0
	dataDir := t.TempDir()
	storage := opencode.NewStorage(dataDir)
	now := time.Now()
	oldCosts := opencode.CostsResult{Usage: []opencode.MonthlyCostItem{{Model: "old", TotalCost: 1}}, Keys: []opencode.KeyInfo{}}
	if err := storage.SaveCosts(now.Year(), int(now.Month()), oldCosts); err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{Transport: syncerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.Header.Get("X-Server-Id") {
		case opencode.FnUsageHistory:
			calls = append(calls, "history")
			usageCalls++
			if usageCalls == 1 {
				return syncerRPCResponse([]opencode.UsageRecord{{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z", Model: "test-model", Provider: "test-provider"}}), nil
			}
			return syncerRPCResponse([]opencode.UsageRecord{}), nil
		case opencode.FnMonthlyCosts:
			calls = append(calls, "cost")
			return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("temporary failure")), Header: make(http.Header)}, nil
		default:
			return nil, errors.New("unexpected RPC")
		}
	})}

	result, err := runWithHTTPClient(Options{DataDir: dataDir}, httpClient)
	if err != nil {
		t.Fatalf("history should succeed when costs fail: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(encoded), `"status":"partial"`) {
		t.Fatalf("partial sync result = %s, err = %v", encoded, err)
	}
	if len(calls) < 2 || calls[0] != "history" || calls[len(calls)-1] != "cost" {
		t.Fatalf("sync order = %v, want history before cost", calls)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("partial cost sync must include a warning")
	}
	history, err := storage.LoadHistory()
	if err != nil || len(history.Records) != 1 || history.Records[0].ID != "usg_new" {
		t.Fatalf("history after partial cost sync = %+v, err = %v", history, err)
	}
	costs, err := storage.GetCosts(now.Year(), int(now.Month()))
	if err != nil || costs == nil || len(costs.Usage) != 1 || costs.Usage[0].Model != "old" {
		t.Fatalf("costs after partial sync = %+v, err = %v", costs, err)
	}
}

func TestRunReportsPartialWhenCostsCannotBeSaved(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dataDir, "costs.json"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "wrk_test")
	httpClient := &http.Client{Transport: syncerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.Header.Get("X-Server-Id") {
		case opencode.FnUsageHistory:
			return syncerRPCResponse([]opencode.UsageRecord{}), nil
		case opencode.FnMonthlyCosts:
			return syncerRPCResponse(map[string]any{"usage": []any{}, "keys": []any{}}), nil
		default:
			return nil, errors.New("unexpected RPC")
		}
	})}

	result, err := runWithHTTPClient(Options{DataDir: dataDir}, httpClient)
	if err != nil || result.Status != opencode.SyncPartial || len(result.Warnings) == 0 {
		t.Fatalf("cost write failure = %+v, err = %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "history.json")); err != nil {
		t.Fatalf("history was not committed: %v", err)
	}
}

func TestRunAutomaticallySelectsSingleWorkspace(t *testing.T) {
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "")
	t.Setenv("OPENCODE_WORKSPACE", "")
	httpClient := &http.Client{Transport: syncerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
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
	})}

	result, err := runWithHTTPClient(Options{DataDir: t.TempDir()}, httpClient)
	if err != nil {
		t.Fatalf("single workspace should be selected automatically: %v", err)
	}
	if result.Status != opencode.SyncComplete || len(result.Warnings) != 0 {
		t.Fatalf("complete sync result = %+v", result)
	}
}

func TestRunFailsWhenNoWorkspaceAvailable(t *testing.T) {
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "")
	t.Setenv("OPENCODE_WORKSPACE", "")
	httpClient := &http.Client{Transport: syncerRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return syncerRPCResponse(map[string]any{"workspaces": []any{}}), nil
	})}

	if _, err := runWithHTTPClient(Options{DataDir: t.TempDir()}, httpClient); err == nil || !strings.Contains(err.Error(), "无法自动发现工作区") {
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

	if _, err := Run(Options{DataDir: dataDir}); err == nil || opencode.SyncErrorReasonOf(err) != opencode.SyncReasonConflict {
		t.Fatalf("locked sync must return a conflict, got %v", err)
	}
}

func TestRunRequiresExplicitWorkspaceWhenMultiple(t *testing.T) {
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "")
	t.Setenv("OPENCODE_WORKSPACE", "")
	httpClient := &http.Client{Transport: syncerRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return syncerRPCResponse(map[string]any{
			"workspaces": []any{
				map[string]any{"id": "wrk_one", "name": "One"},
				map[string]any{"id": "wrk_two", "name": "Two"},
			},
		}), nil
	})}

	if _, err := runWithHTTPClient(Options{DataDir: t.TempDir()}, httpClient); err == nil || !strings.Contains(err.Error(), "多个") {
		t.Fatalf("multiple workspaces must require an explicit selection, got %v", err)
	}
}
