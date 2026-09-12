package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/opencode-analyzer/internal/opencode"
)

type serverRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn serverRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestListenAndServeRejectsNonLoopback(t *testing.T) {
	server := NewServer("", Options{DataDir: t.TempDir()})
	err := server.ListenAndServe("0.0.0.0:not-a-port")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "loopback") {
		t.Fatalf("non-loopback address must be rejected explicitly, got %v", err)
	}
}

func TestListenAndServeAcceptsLoopbackAddress(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:50800", "[::1]:50800", "localhost:50800"} {
		if err := validateLoopbackAddr(addr); err != nil {
			t.Fatalf("loopback address %q was rejected: %v", addr, err)
		}
	}
}

func TestSyncReturnsConflictWhenDataDirectoryIsLocked(t *testing.T) {
	dataDir := t.TempDir()
	storage := opencode.NewStorage(dataDir)
	unlock, err := storage.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "wrk_test")

	handler := NewServer("", Options{DataDir: dataDir}).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/opencode/sync", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "同步进行中") || !strings.Contains(response.Body.String(), `"error":"Conflict"`) || !strings.Contains(response.Body.String(), `"reason":"conflict"`) || !strings.Contains(response.Body.String(), `"status":"failed"`) || !strings.Contains(response.Body.String(), `"added":0`) {
		t.Fatalf("locked sync = %d %s", response.Code, response.Body.String())
	}
}

func TestSecondConcurrentLockFailsAndReleaseAllowsNext(t *testing.T) {
	storage := opencode.NewStorage(t.TempDir())
	firstUnlock, err := storage.Lock()
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			firstUnlock()
		}
	}()

	result := make(chan error, 1)
	go func() {
		secondUnlock, err := storage.Lock()
		if err == nil {
			secondUnlock()
		}
		result <- err
	}()
	if err := <-result; err == nil || !strings.Contains(err.Error(), "同步进行中") {
		t.Fatalf("concurrent lock result = %v", err)
	}

	firstUnlock()
	released = true
	thirdUnlock, err := storage.Lock()
	if err != nil {
		t.Fatal(err)
	}
	thirdUnlock()
}

func TestSyncFailureIncludesFailedStatus(t *testing.T) {
	t.Setenv("OPENCODE_AUTH", "test-cookie")
	t.Setenv("OPENCODE_WORKSPACE_ID", "wrk_test")
	previousTransport := http.DefaultTransport
	http.DefaultTransport = serverRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("failed")), Header: make(http.Header)}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	handler := NewServer("", Options{DataDir: t.TempDir()}).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/opencode/sync", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"status":"failed"`) || !strings.Contains(response.Body.String(), `"reason":"server"`) {
		t.Fatalf("failed sync = %d %s", response.Code, response.Body.String())
	}
}

func TestSyncRejectsNegativeLimitAsBadRequest(t *testing.T) {
	handler := NewServer("", Options{DataDir: t.TempDir()}).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/opencode/sync", strings.NewReader(`{"limit":-1}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "Bad Request") {
		t.Fatalf("negative limit = %d %s", response.Code, response.Body.String())
	}
}

func TestStandaloneHTTPAPIAndWebUI(t *testing.T) {
	dataDir := t.TempDir()
	piDir := t.TempDir()
	pi := `{"type":"session","id":"s1","timestamp":"2026-09-01T00:00:00Z","cwd":"/repo"}
{"type":"message","timestamp":"2026-09-01T01:00:00Z","message":{"role":"assistant","usage":{"input":10,"output":5,"cacheRead":2,"cost":{"total":0.01}}}}
`
	if err := os.WriteFile(filepath.Join(piDir, "s1.jsonl"), []byte(pi), 0644); err != nil {
		t.Fatal(err)
	}
	storage := opencode.NewStorage(dataDir)
	record := opencode.UsageRecord{ID: "usg_1", TimeCreated: "2026-09-01T02:00:00Z", InputTokens: 20, OutputTokens: 10, Cost: 0.02}
	if err := storage.SaveHistory([]opencode.UsageRecord{record}, &record.TimeCreated); err != nil {
		t.Fatal(err)
	}

	handler := NewServer(piDir, Options{DataDir: dataDir}).Handler()
	for _, path := range []string{"/", "/api/opencode/costs?year=2026&month=9", "/api/opencode/history?page=1&size=20", "/api/opencode/audit?year=2026&month=9"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", path, response.Code, response.Body.String())
		}
	}

	auditRequest := httptest.NewRequest(http.MethodGet, "/api/opencode/audit?year=2026&month=9", nil)
	auditResponse := httptest.NewRecorder()
	handler.ServeHTTP(auditResponse, auditRequest)
	var audit opencode.AuditResult
	if err := json.Unmarshal(auditResponse.Body.Bytes(), &audit); err != nil {
		t.Fatal(err)
	}
	if audit.LocalTotals.Requests != 1 || audit.OpencodeTotals.Requests != 1 {
		t.Fatalf("audit = %+v", audit)
	}

	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(index.Body.String(), "OpenCode Analyzer") || !strings.Contains(index.Body.String(), "/api/opencode/history") {
		t.Fatal("standalone WebUI is missing OpenCode audit markers")
	}
	if !strings.Contains(index.Body.String(), "r.status") || !strings.Contains(index.Body.String(), "r.warnings") || !strings.Contains(index.Body.String(), "e.status") {
		t.Fatal("standalone WebUI does not expose sync status warnings")
	}

	unsupported := httptest.NewRecorder()
	handler.ServeHTTP(unsupported, httptest.NewRequest(http.MethodGet, "/api/totals", nil))
	if unsupported.Code != http.StatusNotFound {
		t.Fatalf("token-analyzer route leaked into standalone server: %d", unsupported.Code)
	}
}
