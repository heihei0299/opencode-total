package opencode

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func rpcResponse(value any) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(EncodePayload([]any{value}))),
		Header:     make(http.Header),
	}
}

func TestClientRPCAndPagination(t *testing.T) {
	client := NewClientWithHTTPClient("test-cookie", &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if !strings.Contains(request.Header.Get("Cookie"), "auth=test-cookie") {
			t.Fatalf("missing auth cookie: %q", request.Header.Get("Cookie"))
		}
		switch request.Header.Get("X-Server-Id") {
		case FnWorkspaces:
			if request.Method != http.MethodPost {
				t.Fatalf("workspaces method = %s", request.Method)
			}
			return rpcResponse(map[string]any{"workspaces": []any{map[string]any{"id": "wrk_test", "name": "Test"}}}), nil
		case FnMonthlyCosts:
			if !strings.Contains(request.URL.Query().Get("args"), "2026") {
				t.Fatalf("monthly args = %q", request.URL.Query().Get("args"))
			}
			return rpcResponse(map[string]any{"usage": []any{}, "keys": []any{}}), nil
		case FnUsageHistory:
			if !strings.Contains(request.URL.Query().Get("args"), "0") {
				t.Fatalf("usage args = %q", request.URL.Query().Get("args"))
			}
			return rpcResponse(map[string]any{"records": []any{map[string]any{"id": "usg_1", "workspaceID": "wrk_test"}, map[string]any{"id": "ignored"}}}), nil
		default:
			t.Fatalf("unexpected function id: %s", request.Header.Get("X-Server-Id"))
			return nil, nil
		}
	})})

	workspaces, err := client.GetWorkspaces()
	if err != nil || len(workspaces) != 1 || workspaces[0].ID != "wrk_test" {
		t.Fatalf("workspaces = %+v, err = %v", workspaces, err)
	}
	if _, err := client.GetMonthlyCosts("wrk_test", 2026, 9); err != nil {
		t.Fatalf("monthly costs: %v", err)
	}
	records, err := client.GetUsageHistory("wrk_test", 0)
	if err != nil || len(records) != 1 || records[0].ID != "usg_1" {
		t.Fatalf("records = %+v, err = %v", records, err)
	}
}

func TestUsageHistoryFallsBackToWorkspaceHTML(t *testing.T) {
	client := NewClientWithHTTPClient("test-cookie", &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/_server" {
			return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("failed")), Header: make(http.Header)}, nil
		}
		html := `<script>$R[1]={id:"usg_html",workspaceID:"wrk_test",timeCreated:new Date("2026-09-01T00:00:00Z"),timeUpdated:"2026-09-01T00:00:00Z",model:"m1",provider:"p1",inputTokens:12,outputTokens:3,cost:0.01,keyID:"key_1",sessionID:"session_1"}</script>`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(html)), Header: make(http.Header)}, nil
	})})
	records, err := client.GetUsageHistory("wrk_test", 0)
	if err != nil || len(records) != 1 || records[0].ID != "usg_html" || records[0].InputTokens != 12 {
		t.Fatalf("records = %+v, err = %v", records, err)
	}
}

func TestClientRejectsMissingCredentials(t *testing.T) {
	client := NewClient("")
	if _, err := client.GetWorkspaces(); err == nil || !strings.Contains(err.Error(), "认证失效") {
		t.Fatalf("expected credential error, got %v", err)
	}
}
