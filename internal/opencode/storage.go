package opencode

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type HistoryFile struct {
	Records        []UsageRecord `json:"records"`
	LastSyncedTime *string       `json:"lastSyncedTime"`
	UpdatedAt      string        `json:"updatedAt"`
}

type Storage struct {
	dataDir string
}

func NewStorage(dataDir string) *Storage {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "data/opencode"
	}
	return &Storage{dataDir: dataDir}
}

func (s *Storage) DataDir() string { return s.dataDir }

func (s *Storage) EnsureDataDir() error {
	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(s.costsPath()); os.IsNotExist(err) {
		if err := os.WriteFile(s.costsPath(), []byte("{}\n"), 0644); err != nil {
			return err
		}
	}
	if _, err := os.Stat(s.historyPath()); os.IsNotExist(err) {
		history := HistoryFile{Records: []UsageRecord{}, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		data, err := json.MarshalIndent(history, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(s.historyPath(), append(data, '\n'), 0644); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) costsPath() string   { return filepath.Join(s.dataDir, "costs.json") }
func (s *Storage) historyPath() string { return filepath.Join(s.dataDir, "history.json") }
func (s *Storage) csvPath() string     { return filepath.Join(s.dataDir, "history.csv") }
func (s *Storage) lockPath() string    { return filepath.Join(s.dataDir, ".lock") }

// Lock acquires a cross-process exclusive lock for sync writes.
func (s *Storage) Lock() (func(), error) {
	if err := s.EnsureDataDir(); err != nil {
		return nil, err
	}
	return lockFile(s.lockPath())
}

func (s *Storage) readCosts() (map[string]CostsResult, error) {
	data, err := os.ReadFile(s.costsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]CostsResult{}, nil
		}
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if entries, ok := raw["entries"]; ok {
		var nested map[string]CostsResult
		if err := json.Unmarshal(entries, &nested); err == nil {
			return nested, nil
		}
	}
	out := make(map[string]CostsResult)
	for key, value := range raw {
		var result CostsResult
		if err := json.Unmarshal(value, &result); err == nil {
			out[key] = result
		}
	}
	return out, nil
}

func (s *Storage) GetCosts(year, month int) (*CostsResult, error) {
	costs, err := s.readCosts()
	if err != nil {
		return nil, err
	}
	result, ok := costs[fmt.Sprintf("%04d-%02d", year, month)]
	if !ok {
		return nil, nil
	}
	return &result, nil
}

func (s *Storage) SaveCosts(year, month int, result CostsResult) error {
	if err := s.EnsureDataDir(); err != nil {
		return err
	}
	costs, err := s.readCosts()
	if err != nil {
		return err
	}
	costs[fmt.Sprintf("%04d-%02d", year, month)] = result
	data, err := json.MarshalIndent(costs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.costsPath(), append(data, '\n'), 0644)
}

func (s *Storage) ListCosts() ([]struct {
	Year   int
	Month  int
	Result CostsResult
}, error) {
	costs, err := s.readCosts()
	if err != nil {
		return nil, err
	}
	out := make([]struct {
		Year   int
		Month  int
		Result CostsResult
	}, 0, len(costs))
	for key, result := range costs {
		var year, month int
		if _, err := fmt.Sscanf(key, "%d-%d", &year, &month); err != nil || month < 1 || month > 12 {
			continue
		}
		out = append(out, struct {
			Year   int
			Month  int
			Result CostsResult
		}{Year: year, Month: month, Result: result})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Year != out[j].Year {
			return out[i].Year < out[j].Year
		}
		return out[i].Month < out[j].Month
	})
	return out, nil
}

func (s *Storage) LoadHistory() (*HistoryFile, error) {
	data, err := os.ReadFile(s.historyPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &HistoryFile{Records: []UsageRecord{}}, nil
		}
		return nil, err
	}
	var history HistoryFile
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	if history.Records == nil {
		history.Records = []UsageRecord{}
	}
	if history.UpdatedAt == "" {
		history.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return &history, nil
}

func (s *Storage) SaveHistory(records []UsageRecord, lastSyncedTime *string) error {
	if err := s.EnsureDataDir(); err != nil {
		return err
	}
	ordered := append([]UsageRecord(nil), records...)
	sortUsage(ordered)
	history := HistoryFile{
		Records:        ordered,
		LastSyncedTime: lastSyncedTime,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.historyPath(), append(data, '\n'), 0644); err != nil {
		return err
	}
	return s.ExportCSV(ordered)
}

func (s *Storage) MergeHistory(records []UsageRecord) (int, error) {
	if err := s.EnsureDataDir(); err != nil {
		return 0, err
	}
	history, err := s.LoadHistory()
	if err != nil {
		return 0, err
	}
	byID := make(map[string]UsageRecord, len(history.Records)+len(records))
	for _, record := range history.Records {
		byID[record.ID] = record
	}
	added := 0
	for _, record := range records {
		if _, exists := byID[record.ID]; !exists {
			added++
		}
		byID[record.ID] = record
	}
	merged := make([]UsageRecord, 0, len(byID))
	for _, record := range byID {
		merged = append(merged, record)
	}
	sortUsage(merged)
	var last *string
	if len(merged) > 0 {
		value := merged[0].TimeCreated
		last = &value
	} else {
		last = history.LastSyncedTime
	}
	if err := s.SaveHistory(merged, last); err != nil {
		return 0, err
	}
	return added, nil
}

func sortUsage(records []UsageRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		left, leftOK := parseTimestamp(records[i].TimeCreated)
		right, rightOK := parseTimestamp(records[j].TimeCreated)
		if leftOK && rightOK && !left.Equal(right) {
			return left.After(right)
		}
		return records[i].TimeCreated > records[j].TimeCreated
	})
}

func EncodeCSV(records []UsageRecord) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	headers := []string{
		"id", "workspaceID", "timeCreated", "timeUpdated", "timeDeleted",
		"model", "provider", "inputTokens", "outputTokens", "reasoningTokens",
		"cacheReadTokens", "cacheWrite5mTokens", "cacheWrite1hTokens", "cost",
		"keyID", "sessionID", "enrichment",
	}
	if err := writer.Write(headers); err != nil {
		return nil, err
	}
	for _, record := range records {
		deleted, session := "", ""
		if record.TimeDeleted != nil {
			deleted = *record.TimeDeleted
		}
		if record.SessionID != nil {
			session = *record.SessionID
		}
		enrichment := ""
		if record.Enrichment != nil {
			data, err := json.Marshal(record.Enrichment)
			if err != nil {
				return nil, err
			}
			enrichment = string(data)
		}
		row := []string{
			record.ID,
			record.WorkspaceID,
			record.TimeCreated,
			record.TimeUpdated,
			deleted,
			record.Model,
			record.Provider,
			formatNumber(record.InputTokens),
			formatNumber(record.OutputTokens),
			formatNumber(record.ReasoningTokens),
			formatNumber(record.CacheReadTokens),
			formatNumber(record.CacheWrite5mTokens),
			formatNumber(record.CacheWrite1hTokens),
			formatNumber(record.Cost),
			record.KeyID,
			session,
			enrichment,
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (s *Storage) ExportCSV(records []UsageRecord) error {
	if err := s.EnsureDataDir(); err != nil {
		return err
	}
	data, err := EncodeCSV(records)
	if err != nil {
		return err
	}
	return os.WriteFile(s.csvPath(), data, 0644)
}

func (s *Storage) GetHistory(filter HistoryFilter, page, size int) ([]UsageRecord, int, error) {
	history, err := s.LoadHistory()
	if err != nil {
		return nil, 0, err
	}
	filtered := make([]UsageRecord, 0, len(history.Records))
	for _, record := range history.Records {
		if filter.Model != "" && record.Model != filter.Model {
			continue
		}
		if filter.SessionID != "" && (record.SessionID == nil || *record.SessionID != filter.SessionID) {
			continue
		}
		if filter.Since != "" {
			if timestamp, ok := parseTimestamp(record.TimeCreated); ok {
				if since, valid := parseTimestamp(filter.Since); valid && timestamp.Before(since) {
					continue
				}
			}
		}
		if filter.Until != "" {
			if timestamp, ok := parseTimestamp(record.TimeCreated); ok {
				if until, valid := parseTimestamp(filter.Until); valid && timestamp.After(until) {
					continue
				}
			}
		}
		filtered = append(filtered, record)
	}
	total := len(filtered)
	if page <= 0 || size <= 0 {
		return filtered, total, nil
	}
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return filtered[start:end], total, nil
}

func (s *Storage) Clear() error {
	if err := s.EnsureDataDir(); err != nil {
		return err
	}
	if err := s.SaveHistory([]UsageRecord{}, nil); err != nil {
		return err
	}
	return os.WriteFile(s.costsPath(), []byte("{}\n"), 0644)
}

func (s *Storage) Reset() error { return s.Clear() }

type UsageClient interface {
	GetUsageHistory(workspaceID string, page int) ([]UsageRecord, error)
}

func (s *Storage) Sync(client UsageClient, options SyncOptions) (SyncResult, error) {
	if strings.TrimSpace(options.WorkspaceID) == "" {
		return SyncResult{}, fmt.Errorf("sync 需要 workspaceId")
	}
	if client == nil {
		return SyncResult{}, fmt.Errorf("sync 需要提供 OpenCode client")
	}
	if options.Limit < 0 {
		return SyncResult{}, fmt.Errorf("无效 limit: %d（需为非负整数）", options.Limit)
	}
	if err := s.EnsureDataDir(); err != nil {
		return SyncResult{}, err
	}
	history, err := s.LoadHistory()
	if err != nil {
		return SyncResult{}, err
	}
	existing := make(map[string]bool, len(history.Records))
	for _, record := range history.Records {
		existing[record.ID] = true
	}
	last, lastOK := parseTimestamp(pointerValue(history.LastSyncedTime))
	collected := make([]UsageRecord, 0)
	pages := 0
	maxPages := options.Limit
	if maxPages == 0 {
		// ponytail: finite safety cap for a broken pagination endpoint; raise only if OpenCode history exceeds 200 pages.
		maxPages = 200
	}
	start := time.Now()
	for page := 0; pages < maxPages; page++ {
		batch, err := client.GetUsageHistory(options.WorkspaceID, page)
		pages++
		if err != nil {
			return SyncResult{}, err
		}
		if len(batch) == 0 {
			break
		}
		if options.Full {
			collected = append(collected, batch...)
			continue
		}
		cutoff := -1
		for i, record := range batch {
			if existing[record.ID] {
				cutoff = i
				break
			}
			if lastOK {
				if timestamp, ok := parseTimestamp(record.TimeCreated); ok && !timestamp.After(last) {
					cutoff = i
					break
				}
			}
		}
		if cutoff >= 0 {
			collected = append(collected, batch[:cutoff]...)
			break
		}
		collected = append(collected, batch...)
	}
	added := 0
	if len(collected) > 0 {
		added, err = s.MergeHistory(collected)
	} else {
		err = s.ExportCSV(history.Records)
	}
	if err != nil {
		return SyncResult{}, err
	}
	after, err := s.LoadHistory()
	if err != nil {
		return SyncResult{}, err
	}
	return SyncResult{
		Added:          added,
		Pages:          pages,
		ElapsedMs:      time.Since(start).Milliseconds(),
		LastSyncedTime: pointerValue(after.LastSyncedTime),
	}, nil
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func parseTimestamp(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return parsed, err == nil
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func CsvEscape(value string) string {
	if strings.ContainsAny(value, "\",\n\r") {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return value
}

func ToFloatStr(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }
