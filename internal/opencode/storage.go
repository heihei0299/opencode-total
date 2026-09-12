package opencode

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

type historySaveResult struct {
	csvErr error
}

type mergeHistoryResult struct {
	added          int
	updated        int
	lastSyncedTime string
	csvErr         error
}

func failedSyncResult(reason SyncErrorReason, err error) (SyncResult, error) {
	err = NewSyncError(reason, err)
	return SyncResult{Status: SyncFailed, Reason: SyncErrorReasonOf(err)}, err
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(tempPath)
	}()
	if err := file.Chmod(perm); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
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
		if err := writeFileAtomic(s.costsPath(), []byte("{}\n"), 0644); err != nil {
			return err
		}
	}
	if _, err := os.Stat(s.historyPath()); os.IsNotExist(err) {
		history := HistoryFile{Records: []UsageRecord{}, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		data, err := json.MarshalIndent(history, "", "  ")
		if err != nil {
			return err
		}
		if err := writeFileAtomic(s.historyPath(), append(data, '\n'), 0644); err != nil {
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
	if raw == nil {
		return nil, fmt.Errorf("成本数据必须是 JSON 对象")
	}
	if entries, ok := raw["entries"]; ok {
		var nested map[string]json.RawMessage
		if err := json.Unmarshal(entries, &nested); err != nil || nested == nil {
			if err == nil {
				err = fmt.Errorf("entries 不能为 null")
			}
			return nil, fmt.Errorf("无效成本数据 entries: %w", err)
		}
		return decodeCostsMap(nested)
	}
	return decodeCostsMap(raw)
}

func decodeCostsMap(raw map[string]json.RawMessage) (map[string]CostsResult, error) {
	if raw == nil {
		return nil, fmt.Errorf("成本数据必须是 JSON 对象")
	}
	out := make(map[string]CostsResult, len(raw))
	for key, value := range raw {
		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil {
			return nil, fmt.Errorf("无效成本数据 %s: %w", key, err)
		}
		result, err := decodeCostsResult(decoded)
		if err != nil {
			return nil, fmt.Errorf("无效成本数据 %s: %w", key, err)
		}
		out[key] = result
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
	return writeFileAtomic(s.costsPath(), append(data, '\n'), 0644)
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
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(data, &shape); err != nil {
		return nil, err
	}
	if shape == nil {
		return nil, fmt.Errorf("历史数据必须是 JSON 对象")
	}
	recordsRaw, ok := shape["records"]
	if !ok || strings.TrimSpace(string(recordsRaw)) == "null" {
		return nil, fmt.Errorf("历史数据缺少 records 数组")
	}
	var records []UsageRecord
	if err := json.Unmarshal(recordsRaw, &records); err != nil {
		return nil, fmt.Errorf("无效 records 数组: %w", err)
	}
	var history HistoryFile
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	if err := validateStoredHistoryRecords(records); err != nil {
		return nil, err
	}
	history.Records = records
	if history.Records == nil {
		history.Records = []UsageRecord{}
	}
	if history.UpdatedAt == "" {
		history.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return &history, nil
}

func validateStoredHistoryRecords(records []UsageRecord) error {
	for index, record := range records {
		if strings.TrimSpace(record.ID) == "" {
			return fmt.Errorf("历史记录 %d 缺少 id", index)
		}
		if strings.TrimSpace(record.TimeCreated) == "" {
			if strings.TrimSpace(record.Model) == "" && strings.TrimSpace(record.Provider) == "" {
				continue
			}
			return fmt.Errorf("历史记录 %s 缺少 timeCreated", record.ID)
		}
	}
	return nil
}

func (s *Storage) SaveHistory(records []UsageRecord, lastSyncedTime *string) error {
	result, err := s.saveHistory(records, lastSyncedTime)
	if err != nil {
		return err
	}
	return result.csvErr
}

func (s *Storage) saveHistory(records []UsageRecord, lastSyncedTime *string) (historySaveResult, error) {
	ordered := append([]UsageRecord(nil), records...)
	if err := validateStoredHistoryRecords(ordered); err != nil {
		return historySaveResult{}, err
	}
	if err := s.EnsureDataDir(); err != nil {
		return historySaveResult{}, err
	}
	sortUsage(ordered)
	history := HistoryFile{
		Records:        ordered,
		LastSyncedTime: lastSyncedTime,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return historySaveResult{}, err
	}
	if err := writeFileAtomic(s.historyPath(), append(data, '\n'), 0644); err != nil {
		return historySaveResult{}, err
	}
	return historySaveResult{csvErr: s.ExportCSV(ordered)}, nil
}

func (s *Storage) MergeHistory(records []UsageRecord) (int, error) {
	result, err := s.mergeHistory(records)
	if err != nil {
		return 0, err
	}
	return result.added, result.csvErr
}

func (s *Storage) mergeHistory(records []UsageRecord) (mergeHistoryResult, error) {
	if _, err := validateUsageRecords(records); err != nil {
		return mergeHistoryResult{}, err
	}
	if err := s.EnsureDataDir(); err != nil {
		return mergeHistoryResult{}, err
	}
	history, err := s.LoadHistory()
	if err != nil {
		return mergeHistoryResult{}, err
	}
	byID := make(map[string]UsageRecord, len(history.Records)+len(records))
	for _, record := range history.Records {
		byID[record.ID] = record
	}
	result := mergeHistoryResult{}
	for _, record := range records {
		if existing, exists := byID[record.ID]; !exists {
			result.added++
		} else if !reflect.DeepEqual(existing, record) {
			result.updated++
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
	saved, err := s.saveHistory(merged, last)
	if err != nil {
		return mergeHistoryResult{}, err
	}
	result.csvErr = saved.csvErr
	result.lastSyncedTime = pointerValue(last)
	return result, nil
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
	return writeFileAtomic(s.csvPath(), data, 0644)
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
	return writeFileAtomic(s.costsPath(), []byte("{}\n"), 0644)
}

func (s *Storage) Reset() error { return s.Clear() }

type UsageClient interface {
	GetUsageHistory(workspaceID string, page int) ([]UsageRecord, error)
}

func (s *Storage) Sync(client UsageClient, options SyncOptions) (SyncResult, error) {
	if strings.TrimSpace(options.WorkspaceID) == "" {
		return failedSyncResult(SyncReasonInvalidInput, fmt.Errorf("sync 需要 workspaceId"))
	}
	if client == nil {
		return failedSyncResult(SyncReasonInvalidInput, fmt.Errorf("sync 需要提供 OpenCode client"))
	}
	if options.Limit < 0 {
		return failedSyncResult(SyncReasonInvalidInput, fmt.Errorf("无效 limit: %d（需为非负整数）", options.Limit))
	}
	if err := s.EnsureDataDir(); err != nil {
		return failedSyncResult(SyncReasonStorage, err)
	}
	history, err := s.LoadHistory()
	if err != nil {
		return failedSyncResult(SyncReasonStorage, err)
	}
	existing := make(map[string]bool, len(history.Records))
	for _, record := range history.Records {
		existing[record.ID] = true
	}
	last, lastOK := parseTimestamp(pointerValue(history.LastSyncedTime))
	collected := make([]UsageRecord, 0)
	pages := 0
	failWithProgress := func(reason SyncErrorReason, err error) (SyncResult, error) {
		result, err := failedSyncResult(reason, err)
		result.Pages = pages
		result.LastSyncedTime = pointerValue(history.LastSyncedTime)
		return result, err
	}
	completed := false
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
			return failWithProgress(SyncReasonRemote, err)
		}
		if len(batch) == 0 {
			completed = true
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
			collected = append(collected, batch...)
			completed = true
			break
		}
		collected = append(collected, batch...)
	}
	if !completed {
		return failWithProgress(SyncReasonIncomplete, fmt.Errorf("sync 未完成：达到页数上限前未遇到结束位置"))
	}
	added, updated := 0, 0
	lastSyncedTime := pointerValue(history.LastSyncedTime)
	historyCommitted := false
	status := SyncComplete
	warnings := []string{}
	if len(collected) > 0 {
		merged, mergeErr := s.mergeHistory(collected)
		if mergeErr != nil {
			return failWithProgress(SyncReasonStorage, mergeErr)
		}
		added = merged.added
		updated = merged.updated
		lastSyncedTime = merged.lastSyncedTime
		historyCommitted = true
		if merged.csvErr != nil {
			status = SyncPartial
			warnings = append(warnings, fmt.Sprintf("派生 CSV 未更新: %v", merged.csvErr))
		}
	} else if csvErr := s.ExportCSV(history.Records); csvErr != nil {
		status = SyncPartial
		warnings = append(warnings, fmt.Sprintf("派生 CSV 未更新: %v", csvErr))
	}
	after, err := s.LoadHistory()
	if err != nil {
		if historyCommitted {
			warnings = append(warnings, fmt.Sprintf("历史提交后回读失败: %v", err))
			return SyncResult{
				Added:          added,
				Updated:        updated,
				Pages:          pages,
				ElapsedMs:      time.Since(start).Milliseconds(),
				LastSyncedTime: lastSyncedTime,
				Status:         SyncPartial,
				Reason:         SyncReasonStorage,
				Warnings:       warnings,
			}, nil
		}
		return failWithProgress(SyncReasonStorage, err)
	}
	return SyncResult{
		Added:          added,
		Updated:        updated,
		Pages:          pages,
		ElapsedMs:      time.Since(start).Milliseconds(),
		LastSyncedTime: pointerValue(after.LastSyncedTime),
		Status:         status,
		Warnings:       warnings,
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
