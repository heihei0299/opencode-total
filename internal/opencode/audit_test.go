package opencode

import (
	"testing"
)

func TestOpencodeAudit(t *testing.T) {
	records := []UsageRecord{
		{
			InputTokens:     100,
			OutputTokens:    50,
			CacheReadTokens: 20,
			Cost:            891912946, // scaled integer -> 8.91912946
		},
		{
			InputTokens:        200,
			OutputTokens:       100,
			CacheWrite5mTokens: 10,
			CacheWrite1hTokens: 20,
			Cost:               0.05, // floating direct
		},
	}

	totals := ComputeOpencodeTotals(records)
	if totals.Requests != 2 {
		t.Errorf("expected 2 requests, got %d", totals.Requests)
	}
	// totalTokens = input (300) + cacheRead (20) + output (150) = 470
	if totals.TotalTokens != 470 {
		t.Errorf("expected 470 totalTokens, got %f", totals.TotalTokens)
	}
	if totals.CacheWrite != 30 {
		t.Errorf("expected 30 cacheWrite, got %f", totals.CacheWrite)
	}

	expectedCost := (891912946.0 / 1e8) + 0.05
	if totals.Cost < expectedCost-1e-6 || totals.Cost > expectedCost+1e-6 {
		t.Errorf("expected %f cost, got %f", expectedCost, totals.Cost)
	}

	local := LocalTotals{
		Requests:    1,
		TotalTokens: 400,
		Cost:        5.0,
	}

	audit := BuildAudit(local, records, 2026, 8)
	if audit.Diff.Requests != 1 {
		t.Errorf("expected diff requests = 1, got %d", audit.Diff.Requests)
	}
	if audit.Diff.Tokens != 70 {
		t.Errorf("expected diff tokens = 70, got %f", audit.Diff.Tokens)
	}
}
