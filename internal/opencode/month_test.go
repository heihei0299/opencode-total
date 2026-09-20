package opencode

import (
	"testing"
	"time"
)

func TestParseMonth(t *testing.T) {
	for _, test := range []struct {
		value       string
		year, month int
		ok          bool
	}{
		{value: "2026-09", year: 2026, month: 9, ok: true},
		{value: "2026-00"},
		{value: "2026-13"},
		{value: "26-09"},
	} {
		year, month, ok := ParseMonth(test.value)
		if year != test.year || month != test.month || ok != test.ok {
			t.Fatalf("ParseMonth(%q) = %d-%02d, %t", test.value, year, month, ok)
		}
	}
}

func TestRecordsInMonthUsesTimestampAndLocation(t *testing.T) {
	loc := time.FixedZone("UTC-7", -7*60*60)
	records := []UsageRecord{
		{ID: "before", TimeCreated: "2026-03-01T06:59:59.999999999Z"},
		{ID: "start", TimeCreated: "2026-03-01T07:00:00Z"},
		{ID: "end", TimeCreated: "2026-04-01T06:59:59.999999999Z"},
		{ID: "after", TimeCreated: "2026-04-01T07:00:00Z"},
		{ID: "invalid", TimeCreated: "not-a-timestamp"},
	}
	filtered := RecordsInMonth(records, 2026, 3, loc)
	if len(filtered) != 2 || filtered[0].ID != "start" || filtered[1].ID != "end" {
		t.Fatalf("filtered records = %+v", filtered)
	}
}
