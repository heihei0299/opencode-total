package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/opencode-analyzer/internal/opencode"
)

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

	unsupported := httptest.NewRecorder()
	handler.ServeHTTP(unsupported, httptest.NewRequest(http.MethodGet, "/api/totals", nil))
	if unsupported.Code != http.StatusNotFound {
		t.Fatalf("token-analyzer route leaked into standalone server: %d", unsupported.Code)
	}
}
