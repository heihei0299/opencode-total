package opencode

import (
	"strconv"
	"strings"
	"time"
)

func ParseMonth(value string) (year, month int, ok bool) {
	if len(value) != 7 || value[4] != '-' {
		return 0, 0, false
	}
	year, yearErr := strconv.Atoi(value[:4])
	month, monthErr := strconv.Atoi(value[5:])
	if yearErr != nil || year < 1000 || monthErr != nil || month < 1 || month > 12 {
		return 0, 0, false
	}
	return year, month, true
}

func MonthBounds(year, month int, loc *time.Location) (time.Time, time.Time) {
	if loc == nil {
		loc = time.Local
	}
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 1, 0).Add(-time.Nanosecond)
}

func RecordsInMonth(records []UsageRecord, year, month int, loc *time.Location) []UsageRecord {
	start, end := MonthBounds(year, month, loc)
	out := make([]UsageRecord, 0, len(records))
	for _, record := range records {
		timestamp, ok := ParseTimestamp(record.TimeCreated)
		if !ok || timestamp.Before(start) || timestamp.After(end) {
			continue
		}
		out = append(out, record)
	}
	return out
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
