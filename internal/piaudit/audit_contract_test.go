package piaudit

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTotalsForRangeUsesStandalonePiContract(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.jsonl")
	main := `{"type":"session","id":"main","timestamp":"2026-09-01T00:00:00Z","cwd":"/repo"}
{"type":"message","id":"assistant-1","timestamp":"2026-09-10T01:00:00Z","message":{"role":"assistant","provider":"p","model":"m1","usage":{"input":10,"output":5,"cacheRead":2,"cacheWrite":1,"reasoning":3,"cost":{"total":0.1}}}}
{"type":"message","id":"assistant-1","timestamp":"2026-09-10T01:00:00Z","message":{"role":"assistant","provider":"p","model":"m1","usage":{"input":10,"output":8,"cacheRead":2,"cacheWrite":1,"reasoning":3,"cost":{"total":0.1}},"stopReason":"stop"}}
{"type":"message","id":"tool-1","timestamp":"2026-09-10T02:00:00Z","message":{"role":"toolResult","usage":{"input":2,"output":1}}}
{"type":"compaction","id":"compact-1","timestamp":"2026-09-11T01:00:00Z","usage":{"input":3,"output":4,"cacheRead":1,"cacheWrite":2}}
{"type":"branch_summary","id":"branch-1","timestamp":"2026-09-11T02:00:00Z","usage":{"input":4,"output":2,"cacheWrite":1}}
{"type":"message","id":"failed-1","timestamp":"2026-09-11T03:00:00Z","message":{"role":"assistant","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"cost":{"total":0}},"stopReason":"error"}}
{"type":"message","id":"aborted-1","timestamp":"2026-09-11T03:30:00Z","message":{"role":"assistant","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"cost":{"total":0}},"stopReason":"aborted"}}
{"type":"message","timestamp":"2026-09-11T04:00:00Z","message":{"role":"assistant","model":"m1","usage":{"input":5,"output":1},"stopReason":"stop","irrelevant":"first"}}
{"type":"message","timestamp":"2026-09-11T04:00:00Z","message":{"role":"assistant","model":"m1","usage":{"input":5,"output":1},"stopReason":"stop","irrelevant":"second"}}
{"type":"message","id":"outside-month","timestamp":"2026-10-02T00:00:00Z","message":{"role":"assistant","usage":{"input":100,"output":100},"stopReason":"stop"}}
`
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	forkPath := filepath.Join(dir, "fork.jsonl")
	fork := `{"type":"session","id":"fork","timestamp":"2026-09-12T00:00:00Z","parentSession":"/tmp/main.jsonl"}
{"type":"message","id":"copied","timestamp":"2026-09-11T23:59:59Z","message":{"role":"assistant","usage":{"input":999,"output":999},"stopReason":"stop"}}
{"type":"message","id":"fork-1","timestamp":"2026-09-12T01:00:00Z","message":{"role":"assistant","usage":{"input":6,"output":2},"stopReason":"stop"}}
`
	if err := os.WriteFile(forkPath, []byte(fork), 0o644); err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)
	totals, err := TotalsForRange(dir, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if totals.Requests != 8 || totals.Input != 30 || totals.Output != 18 || totals.CacheRead != 3 || totals.CacheWrite != 4 || totals.Reasoning != 3 || totals.TotalTokens != 51 || math.Abs(totals.Cost-0.1) > 1e-9 {
		t.Fatalf("standalone Pi audit contract drifted: %+v", totals)
	}
	monthTotals, err := TotalsForMonth(dir, 2026, 9)
	if err != nil {
		t.Fatal(err)
	}
	if monthTotals.Requests != totals.Requests || monthTotals.TotalTokens != totals.TotalTokens {
		t.Fatalf("month boundary must match the September range: range=%+v month=%+v", totals, monthTotals)
	}
}

func identityTestEntry(id string, message map[string]any) map[string]any {
	entry := map[string]any{
		"type":      "message",
		"timestamp": "2026-09-10T01:00:00Z",
		"message":   message,
	}
	if id != "" {
		entry["id"] = id
	}
	return entry
}

func identityTestRecord(entry map[string]any, usage, message map[string]any) piAuditRecord {
	identity := makePiAuditIdentity(entry, "assistant", usage, message)
	return piAuditRecord{
		requestID:  identity.requestID,
		semanticID: identity.semanticID,
		hasEntryID: identity.hasEntryID,
		stopReason: stringValue(message["stopReason"]),
		output:     number(usage["output"]),
	}
}

func TestPiAuditIdentityIgnoresIrrelevantMessageMetadata(t *testing.T) {
	usage := map[string]any{"input": 10.0, "output": 5.0, "extra": "same"}
	message1 := map[string]any{"provider": "p", "model": "m", "responseId": "r", "stopReason": "stop", "content": []any{"ok"}, "irrelevant": "first"}
	message2 := map[string]any{"provider": "p", "model": "m", "responseId": "r", "stopReason": "stop", "content": []any{"ok"}, "irrelevant": "second"}

	got := deduplicateRecords([]piAuditRecord{
		identityTestRecord(identityTestEntry("", message1), usage, message1),
		identityTestRecord(identityTestEntry("", message2), usage, message2),
	})
	if len(got) != 1 {
		t.Fatalf("irrelevant message metadata must deduplicate, got %d records", len(got))
	}
}

func TestPiAuditIdentityDistinguishesCanonicalMessageFields(t *testing.T) {
	usage := map[string]any{"input": 10.0, "output": 5.0}
	message1 := map[string]any{"provider": "p", "model": "m", "responseId": "r", "content": []any{"ok"}}
	message2 := map[string]any{"provider": "p", "model": "m2", "responseId": "r", "content": []any{"ok"}}

	got := deduplicateRecords([]piAuditRecord{
		identityTestRecord(identityTestEntry("", message1), usage, message1),
		identityTestRecord(identityTestEntry("", message2), usage, message2),
	})
	if len(got) != 2 {
		t.Fatalf("canonical message changes must not deduplicate, got %d records", len(got))
	}
}

func TestPiAuditIdentityIncludesCompleteUsage(t *testing.T) {
	usage1 := map[string]any{"input": 10.0, "output": 5.0, "metadata": map[string]any{"tier": "standard"}}
	usage2 := map[string]any{"input": 10.0, "output": 5.0, "metadata": map[string]any{"tier": "priority"}}
	message := map[string]any{"provider": "p", "model": "m", "content": []any{"ok"}}

	got := deduplicateRecords([]piAuditRecord{
		identityTestRecord(identityTestEntry("", message), usage1, message),
		identityTestRecord(identityTestEntry("", message), usage2, message),
	})
	if len(got) != 2 {
		t.Fatalf("extra usage fields must distinguish records, got %d", len(got))
	}
}

func TestPiAuditIdentityIgnoresUsageMapKeyOrder(t *testing.T) {
	usage1 := map[string]any{}
	usage1["input"] = 10.0
	usage1["output"] = 5.0
	usage1["cost"] = map[string]any{"total": 0.1, "unit": "usd"}
	usage1["metadata"] = map[string]any{"tier": "standard", "region": "cn"}
	usage2 := map[string]any{}
	usage2["metadata"] = map[string]any{"region": "cn", "tier": "standard"}
	usage2["cost"] = map[string]any{"unit": "usd", "total": 0.1}
	usage2["output"] = 5.0
	usage2["input"] = 10.0
	message := map[string]any{"provider": "p", "model": "m", "content": []any{"ok"}}

	got := deduplicateRecords([]piAuditRecord{
		identityTestRecord(identityTestEntry("", message), usage1, message),
		identityTestRecord(identityTestEntry("", message), usage2, message),
	})
	if len(got) != 1 {
		t.Fatalf("usage map key order must not change identity, got %d records", len(got))
	}
}

func TestPiAuditRequestIDReplacementKeepsFinalRecord(t *testing.T) {
	id := "request-1"
	partialUsage := map[string]any{"input": 10.0, "output": 2.0}
	finalUsage := map[string]any{"input": 10.0, "output": 8.0}
	partialMessage := map[string]any{"provider": "p", "model": "m", "content": []any{"partial"}}
	finalMessage := map[string]any{"provider": "p", "model": "m", "stopReason": "stop", "content": []any{"final"}}

	got := deduplicateRecords([]piAuditRecord{
		identityTestRecord(identityTestEntry(id, partialMessage), partialUsage, partialMessage),
		identityTestRecord(identityTestEntry(id, finalMessage), finalUsage, finalMessage),
	})
	if len(got) != 1 || got[0].output != 8 || got[0].stopReason != "stop" {
		t.Fatalf("request ID replacement must keep final record: %+v", got)
	}
}
