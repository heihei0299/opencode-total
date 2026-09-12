package piaudit

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heihei0299/opencode-analyzer/internal/opencode"
)

func MonthBounds(year, month int, loc *time.Location) (time.Time, time.Time) {
	if loc == nil {
		loc = time.Local
	}
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 1, 0).Add(-time.Nanosecond)
}

// TotalsForMonth reads Pi usage for the standalone audit view. This parser is
// intentionally owned by this module; it does not depend on token-analyzer.
func TotalsForMonth(dir string, year, month int) (opencode.LocalTotals, error) {
	start, end := MonthBounds(year, month, time.Local)
	return TotalsForRange(dir, start, end)
}

func TotalsForRange(dir string, start, end time.Time) (opencode.LocalTotals, error) {
	var totals opencode.LocalTotals
	if dir == "" {
		return totals, nil
	}
	files, err := collectSessionFiles(dir)
	if err != nil {
		return opencode.LocalTotals{}, err
	}
	var records []piAuditRecord
	for _, path := range files {
		fileRecords, err := recordsFromFile(path)
		if err != nil {
			return opencode.LocalTotals{}, err
		}
		records = append(records, fileRecords...)
	}
	for _, record := range deduplicateRecords(records) {
		// Invalid message timestamps are retained conservatively. A valid
		// timestamp is the entry timestamp, matching MessageTimeRange semantics.
		if record.rangeTimestampOK && (record.timestamp.Before(start) || record.timestamp.After(end)) {
			continue
		}
		totals.Requests++
		totals.Input += record.input
		totals.Output += record.output
		totals.CacheRead += record.cacheRead
		totals.CacheWrite += record.cacheWrite
		totals.Reasoning += record.reasoning
		totals.Cost += record.cost
	}
	totals.TotalTokens = totals.Input + totals.CacheRead + totals.Output
	return totals, nil
}

func RecordsInMonth(records []opencode.UsageRecord, year, month int) []opencode.UsageRecord {
	start, end := MonthBounds(year, month, time.Local)
	out := make([]opencode.UsageRecord, 0, len(records))
	for _, record := range records {
		timestamp, ok := ParseTimestamp(record.TimeCreated)
		if !ok || timestamp.Before(start) || timestamp.After(end) {
			continue
		}
		out = append(out, record)
	}
	return out
}

func collectSessionFiles(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	files := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".jsonl" {
			files = append(files, filepath.Join(root, entry.Name()))
		}
	}
	// Support both native Pi layouts without importing token-analyzer discovery.
	if len(files) == 0 {
		for _, project := range entries {
			if !project.IsDir() {
				continue
			}
			projectDir := filepath.Join(root, project.Name())
			projectFiles, readErr := os.ReadDir(projectDir)
			if readErr != nil {
				continue
			}
			for _, entry := range projectFiles {
				if !entry.IsDir() && filepath.Ext(entry.Name()) == ".jsonl" {
					files = append(files, filepath.Join(projectDir, entry.Name()))
				}
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

func ParseTimestamp(value string) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed, true
	}
	parsed, err = time.Parse("2006-01-02T15:04:05.999999999", value)
	return parsed, err == nil
}
