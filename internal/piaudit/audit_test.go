package piaudit

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestTotalsForRangeUsesMessageTimeAndForkCutoff(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "parent.jsonl")
	content := `{"type":"session","id":"parent","timestamp":"2026-09-01T00:00:00Z","cwd":"/repo"}
{"type":"message","timestamp":"2026-09-01T01:00:00Z","message":{"role":"assistant","usage":{"input":100,"output":50,"cacheRead":20,"cacheWrite":10,"reasoning":5,"cost":{"total":0.01}}}}
`
	if err := os.WriteFile(parent, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	fork := filepath.Join(dir, "fork.jsonl")
	forkContent := fmt.Sprintf(`{"type":"session","id":"fork","timestamp":"2026-09-02T00:00:00Z","cwd":"/repo","parentSession":%q}
{"type":"message","timestamp":"2026-09-01T01:00:00Z","message":{"role":"assistant","usage":{"input":1000}}}
{"type":"message","timestamp":"2026-09-02T01:00:00Z","message":{"role":"assistant","usage":{"input":200,"output":25,"cacheRead":5,"cost":{"total":0.02}}}}
`, parent)
	if err := os.WriteFile(fork, []byte(forkContent), 0644); err != nil {
		t.Fatal(err)
	}
	broken := "not-json\n" + content
	if err := os.WriteFile(filepath.Join(dir, "broken.jsonl"), []byte(broken), 0644); err != nil {
		t.Fatal(err)
	}
	start, ok := ParseTimestamp("2026-09-01T00:00:00Z")
	if !ok {
		t.Fatal("invalid start")
	}
	end, ok := ParseTimestamp("2026-09-30T23:59:59Z")
	if !ok {
		t.Fatal("invalid end")
	}
	totals, err := TotalsForRange(dir, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if totals.Requests != 2 || totals.Input != 300 || totals.TotalTokens != 400 || math.Abs(totals.Cost-0.03) > 1e-9 {
		t.Fatalf("totals = %+v", totals)
	}
}
