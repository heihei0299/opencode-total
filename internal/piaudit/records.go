package piaudit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"os"
	"strings"
	"time"
)

type piAuditRecord struct {
	requestID        string
	semanticID       string
	hasEntryID       bool
	stopReason       string
	timestamp        time.Time
	rangeTimestampOK bool
	input            float64
	output           float64
	cacheRead        float64
	cacheWrite       float64
	reasoning        float64
	cost             float64
}

func recordsFromFile(path string) ([]piAuditRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	firstLine := true
	var sessionTimestamp time.Time
	var forkAt *time.Time
	var records []piAuditRecord
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) != nil {
			if firstLine {
				return nil, nil
			}
			continue
		}
		if firstLine {
			firstLine = false
			if stringValue(entry["type"]) != "session" {
				return nil, nil
			}
			sessionTimestamp, _ = parseAnyTimestamp(entry["timestamp"])
			parent := stringValue(entry["parentSession"])
			if strings.Contains(parent, "/") || strings.Contains(parent, "\\") {
				if timestamp, ok := parseAnyTimestamp(entry["timestamp"]); ok {
					forkAt = &timestamp
				}
			}
			continue
		}

		if forkAt != nil {
			if timestamp, ok := parseAnyTimestamp(entry["timestamp"]); ok && timestamp.Before(*forkAt) {
				continue
			}
		}
		if record := parsePiRecord(entry, sessionTimestamp, stat.ModTime()); record != nil {
			records = append(records, *record)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, err
	}
	return records, nil
}

func parsePiRecord(entry map[string]any, sessionTimestamp, fileMtime time.Time) *piAuditRecord {
	typ := stringValue(entry["type"])
	kind := ""
	var usage map[string]any
	var message map[string]any
	stopReason := ""

	switch typ {
	case "message":
		var ok bool
		message, ok = entry["message"].(map[string]any)
		if !ok {
			return nil
		}
		switch stringValue(message["role"]) {
		case "assistant":
			kind = "assistant"
			usage, _ = message["usage"].(map[string]any)
			stopReason = stringValue(message["stopReason"])
		case "toolResult":
			kind = "tool_result"
			usage, _ = message["usage"].(map[string]any)
		default:
			return nil
		}
	case "compaction":
		kind = "compaction"
		usage, _ = entry["usage"].(map[string]any)
	case "branch_summary":
		kind = "branch_summary"
		usage, _ = entry["usage"].(map[string]any)
	default:
		return nil
	}
	if usage == nil {
		return nil
	}

	input := number(usage["input"])
	output := number(usage["output"])
	cacheRead := number(usage["cacheRead"])
	cacheWrite := number(usage["cacheWrite"])
	reasoning := number(usage["reasoning"])
	cost := 0.0
	if costObject, ok := usage["cost"].(map[string]any); ok {
		cost = number(costObject["total"])
	}
	failed := stopReason == "error" || stopReason == "aborted"
	billable := input > 0 || output > 0 || cacheRead > 0 || cacheWrite > 0
	if !billable && cost <= 0 && !failed {
		return nil
	}

	entryTimestamp, entryTimestampOK := parseAnyTimestamp(entry["timestamp"])
	timestamp := entryTimestamp
	if !entryTimestampOK && message != nil {
		timestamp, entryTimestampOK = parseAnyTimestamp(message["timestamp"])
	}
	if timestamp.IsZero() {
		timestamp = sessionTimestamp
	}
	if timestamp.IsZero() {
		timestamp = fileMtime
	}

	identity := makePiAuditIdentity(entry, kind, usage, message)
	return &piAuditRecord{
		requestID:        identity.requestID,
		semanticID:       identity.semanticID,
		hasEntryID:       identity.hasEntryID,
		stopReason:       stopReason,
		timestamp:        timestamp,
		rangeTimestampOK: entryTimestampOK,
		input:            input,
		output:           output,
		cacheRead:        cacheRead,
		cacheWrite:       cacheWrite,
		reasoning:        reasoning,
		cost:             cost,
	}
}

type piAuditIdentity struct {
	requestID  string
	semanticID string
	hasEntryID bool
}

func makePiAuditIdentity(entry map[string]any, kind string, usage, message map[string]any) piAuditIdentity {
	semanticPayload := map[string]any{
		"kind":  kind,
		"usage": usage,
	}
	if timestamp, ok := entry["timestamp"].(string); ok && timestamp != "" {
		semanticPayload["entry_timestamp"] = timestamp
	}
	if message != nil {
		if timestamp, ok := message["timestamp"]; ok {
			semanticPayload["message_timestamp"] = timestamp
		}
		messageFields := make(map[string]any)
		for _, key := range []string{"provider", "model", "responseModel", "responseId", "api", "toolCallId", "toolName", "stopReason", "errorMessage", "content"} {
			if value, ok := message[key]; ok {
				messageFields[key] = value
			}
		}
		semanticPayload["message"] = messageFields
	} else if summary, ok := entry["summary"]; ok {
		semanticPayload["summary"] = summary
	}
	semanticID := "pi_session_semantic:" + digest("pi-session-semantic-v1", semanticPayload)
	entryID := stringValue(entry["id"])
	if entryID == "" {
		return piAuditIdentity{requestID: semanticID, semanticID: semanticID}
	}
	requestID := "pi_session:" + digest("pi-session-request-v3", struct {
		Kind      string `json:"kind"`
		EntryID   string `json:"entryId"`
		Timestamp any    `json:"timestamp"`
	}{kind, entryID, entry["timestamp"]})
	return piAuditIdentity{requestID: requestID, semanticID: semanticID, hasEntryID: true}
}

func digest(label string, value any) string {
	payload, _ := json.Marshal(value)
	h := sha256.New()
	h.Write([]byte(label))
	h.Write([]byte{0})
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func shouldReplaceRecord(existing, candidate piAuditRecord) bool {
	existingFinal := existing.stopReason != ""
	candidateFinal := candidate.stopReason != ""
	if candidateFinal && !existingFinal {
		return true
	}
	if candidateFinal != existingFinal {
		return false
	}
	return candidate.output > existing.output
}

func deduplicateRecords(records []piAuditRecord) []piAuditRecord {
	byRequest := make(map[string]piAuditRecord, len(records))
	order := make([]string, 0, len(records))
	semanticSeen := make(map[string]bool, len(records))
	for _, record := range records {
		if !record.hasEntryID && semanticSeen[record.semanticID] {
			continue
		}
		if existing, ok := byRequest[record.requestID]; ok {
			if shouldReplaceRecord(existing, record) {
				byRequest[record.requestID] = record
				semanticSeen[record.semanticID] = true
			}
			continue
		}
		byRequest[record.requestID] = record
		order = append(order, record.requestID)
		semanticSeen[record.semanticID] = true
	}
	out := make([]piAuditRecord, 0, len(order))
	for _, requestID := range order {
		out = append(out, byRequest[requestID])
	}
	return out
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func parseAnyTimestamp(value any) (time.Time, bool) {
	switch value := value.(type) {
	case string:
		return ParseTimestamp(value)
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return time.Time{}, false
		}
		if value > 1e12 {
			return time.UnixMilli(int64(value)), true
		}
		return time.Unix(int64(value), 0), true
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return time.Time{}, false
		}
		return parseAnyTimestamp(parsed)
	default:
		return time.Time{}, false
	}
}

func number(value any) float64 {
	var result float64
	switch value := value.(type) {
	case float64:
		result = value
	case json.Number:
		result, _ = value.Float64()
	case int:
		result = float64(value)
	case int64:
		result = float64(value)
	}
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0
	}
	return result
}
