package opencode

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeUsageClient map[int][]UsageRecord

func (client fakeUsageClient) GetUsageHistory(_ string, page int) ([]UsageRecord, error) {
	return client[page], nil
}

type laterPageErrorClient struct{}

func (laterPageErrorClient) GetUsageHistory(_ string, page int) ([]UsageRecord, error) {
	if page == 1 {
		return nil, errors.New("later page failed")
	}
	return []UsageRecord{{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z", Model: "test-model", Provider: "test-provider"}}, nil
}

type endlessUsageClient struct {
	calls int
}

func (client *endlessUsageClient) GetUsageHistory(_ string, _ int) ([]UsageRecord, error) {
	client.calls++
	return []UsageRecord{{ID: "usg_page", TimeCreated: "2026-09-01T00:00:00Z", Model: "test-model", Provider: "test-provider"}}, nil
}

func TestStorageSyncPreservesSnapshotOnLaterPageError(t *testing.T) {
	storage := NewStorage(t.TempDir())
	old := UsageRecord{ID: "usg_old", TimeCreated: "2026-09-01T00:00:00Z", Model: "test-model", Provider: "test-provider"}
	if err := storage.SaveHistory([]UsageRecord{old}, &old.TimeCreated); err != nil {
		t.Fatal(err)
	}
	beforeHistory, err := os.ReadFile(storage.historyPath())
	if err != nil {
		t.Fatal(err)
	}
	beforeCSV, err := os.ReadFile(storage.csvPath())
	if err != nil {
		t.Fatal(err)
	}

	result, err := storage.Sync(laterPageErrorClient{}, SyncOptions{WorkspaceID: "wrk_test"})
	if err == nil {
		t.Fatal("later-page errors must fail the sync")
	}
	if result.Status != SyncFailed {
		t.Fatalf("later-page error status = %q, want %q", result.Status, SyncFailed)
	}
	if result.Pages != 2 || result.LastSyncedTime != old.TimeCreated || result.Added != 0 || result.Updated != 0 {
		t.Fatalf("later-page error progress = %+v", result)
	}
	afterHistory, err := os.ReadFile(storage.historyPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(afterHistory) != string(beforeHistory) {
		t.Fatal("later-page error changed history.json")
	}
	afterCSV, err := os.ReadFile(storage.csvPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(afterCSV) != string(beforeCSV) {
		t.Fatal("later-page error changed history.csv")
	}
	history, err := storage.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Records) != 1 || history.Records[0].ID != old.ID || pointerValue(history.LastSyncedTime) != old.TimeCreated {
		t.Fatalf("later-page error changed last successful history: %+v", history)
	}
}

func TestStorageSyncRejectsInternalPageCap(t *testing.T) {
	storage := NewStorage(t.TempDir())
	client := &endlessUsageClient{}
	result, err := storage.Sync(client, SyncOptions{WorkspaceID: "wrk_test"})
	if err == nil {
		t.Fatal("reaching the internal page cap must fail")
	}
	if result.Status != SyncFailed || result.Pages != 200 || result.LastSyncedTime != "" || result.Added != 0 || result.Updated != 0 {
		t.Fatalf("internal cap progress = %+v", result)
	}
	if client.calls != 200 {
		t.Fatalf("internal cap calls = %d, want 200", client.calls)
	}
	history, err := storage.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Records) != 0 || pointerValue(history.LastSyncedTime) != "" {
		t.Fatalf("internal cap changed history: %+v", history)
	}
}

func TestStorageSyncFailsClosedForCorruptHistory(t *testing.T) {
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "history.json")
	original := []byte("not valid json\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	storage := NewStorage(dataDir)
	result, err := storage.Sync(fakeUsageClient{0: {}}, SyncOptions{WorkspaceID: "wrk_test"})
	if err == nil {
		t.Fatal("corrupt history must fail closed")
	}
	if result.Status != SyncFailed {
		t.Fatalf("corrupt history status = %q, want %q", result.Status, SyncFailed)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(original) {
		t.Fatalf("corrupt history was changed: %q", actual)
	}
}

func TestStorageReadsLegacyData(t *testing.T) {
	dataDir := t.TempDir()
	storage := NewStorage(dataDir)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storage.costsPath(), []byte(`{"entries":{"2026-09":{"usage":[],"keys":[]}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storage.historyPath(), []byte(`{"records":[{"id":"usg_legacy"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	costs, err := storage.GetCosts(2026, 9)
	if err != nil || costs == nil {
		t.Fatalf("legacy costs = %+v, err = %v", costs, err)
	}
	history, err := storage.LoadHistory()
	if err != nil || len(history.Records) != 1 || history.Records[0].ID != "usg_legacy" {
		t.Fatalf("legacy history = %+v, err = %v", history, err)
	}
	result, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z", Model: "new-model", Provider: "new-provider"}},
		1: {},
	}, SyncOptions{WorkspaceID: "wrk_test"})
	if err != nil || result.Status != SyncComplete {
		t.Fatalf("legacy history sync = %+v, err = %v", result, err)
	}
	history, err = storage.LoadHistory()
	if err != nil || len(history.Records) != 2 || history.Records[0].ID != "usg_new" || history.Records[1].ID != "usg_legacy" {
		t.Fatalf("merged legacy history = %+v, err = %v", history, err)
	}
}

func TestStorageSyncRejectsInvalidFetchedRecordBeforeWrite(t *testing.T) {
	storage := NewStorage(t.TempDir())
	old := UsageRecord{ID: "usg_old", TimeCreated: "2026-09-01T00:00:00Z", Model: "old-model", Provider: "old-provider"}
	if err := storage.SaveHistory([]UsageRecord{old}, &old.TimeCreated); err != nil {
		t.Fatal(err)
	}
	beforeHistory, err := os.ReadFile(storage.historyPath())
	if err != nil {
		t.Fatal(err)
	}
	beforeCSV, err := os.ReadFile(storage.csvPath())
	if err != nil {
		t.Fatal(err)
	}

	result, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_invalid", TimeCreated: "2026-09-02T00:00:00Z", Model: "new-model"}},
		1: {},
	}, SyncOptions{WorkspaceID: "wrk_test"})
	if err == nil || result.Status != SyncFailed {
		t.Fatalf("invalid fetched record = %+v, err = %v", result, err)
	}
	if result.Pages != 2 || result.LastSyncedTime != old.TimeCreated || result.Added != 0 || result.Updated != 0 {
		t.Fatalf("invalid fetched record progress = %+v", result)
	}
	afterHistory, err := os.ReadFile(storage.historyPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(afterHistory) != string(beforeHistory) {
		t.Fatal("invalid fetched record changed history.json")
	}
	afterCSV, err := os.ReadFile(storage.csvPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(afterCSV) != string(beforeCSV) {
		t.Fatal("invalid fetched record changed history.csv")
	}
}

type finalHistoryReadFailureEnrichment struct {
	path  string
	calls int
}

func (enrichment *finalHistoryReadFailureEnrichment) MarshalJSON() ([]byte, error) {
	enrichment.calls++
	if enrichment.calls == 2 {
		if err := os.Remove(enrichment.path); err != nil {
			return nil, err
		}
		if err := os.Mkdir(enrichment.path, 0755); err != nil {
			return nil, err
		}
	}
	return []byte("null"), nil
}

func TestStorageSyncReportsPartialWhenFinalHistoryReadFails(t *testing.T) {
	dataDir := t.TempDir()
	storage := NewStorage(dataDir)
	old := UsageRecord{ID: "usg_old", TimeCreated: "2026-09-01T00:00:00Z", Model: "old-model", Provider: "old-provider"}
	if err := storage.SaveHistory([]UsageRecord{old}, &old.TimeCreated); err != nil {
		t.Fatal(err)
	}
	enrichment := &finalHistoryReadFailureEnrichment{path: storage.historyPath()}

	result, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z", Model: "new-model", Provider: "new-provider", Enrichment: enrichment}},
		1: {},
	}, SyncOptions{WorkspaceID: "wrk_test"})
	if err != nil || result.Status != SyncPartial {
		t.Fatalf("final history read failure = %+v, err = %v", result, err)
	}
	if result.Pages != 2 || result.LastSyncedTime != "2026-09-02T00:00:00Z" || result.Added != 1 || result.Updated != 0 || result.Reason != SyncReasonStorage {
		t.Fatalf("final history read failure progress = %+v", result)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "历史提交后回读失败") {
		t.Fatalf("final history read failure warnings = %+v", result.Warnings)
	}
	if enrichment.calls != 2 {
		t.Fatalf("history enrichment marshal calls = %d, want 2", enrichment.calls)
	}
}

func TestStorageSyncPreservesHistoryWhenJSONWriteFails(t *testing.T) {
	dataDir := t.TempDir()
	storage := NewStorage(dataDir)
	old := UsageRecord{ID: "usg_old", TimeCreated: "2026-09-01T00:00:00Z", Model: "test-model", Provider: "test-provider"}
	if err := storage.SaveHistory([]UsageRecord{old}, &old.TimeCreated); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dataDir, 0500); err != nil {
		t.Fatal(err)
	}
	probe, probeErr := os.CreateTemp(dataDir, "permission-probe")
	if probeErr == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		_ = os.Chmod(dataDir, 0755)
		t.Skip("filesystem permissions do not prevent writes")
	}
	defer os.Chmod(dataDir, 0755)

	result, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z", Model: "test-model", Provider: "test-provider"}},
		1: {},
	}, SyncOptions{WorkspaceID: "wrk_test"})
	if err == nil || result.Status != SyncFailed {
		t.Fatalf("JSON write failure = %+v, err = %v", result, err)
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old.ID) || strings.Contains(string(data), "usg_new") {
		t.Fatalf("history after JSON write failure = %s", data)
	}
}

func TestStorageSyncKeepsHistoryWhenCSVExportFails(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dataDir, "history.csv"), 0755); err != nil {
		t.Fatal(err)
	}
	storage := NewStorage(dataDir)
	result, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z", Model: "test-model", Provider: "test-provider"}},
		1: {},
	}, SyncOptions{WorkspaceID: "wrk_test"})
	if err != nil {
		t.Fatalf("CSV failure should be partial, got %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("CSV failure must include a warning")
	}
	encoded, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(encoded), `"status":"partial"`) {
		t.Fatalf("partial result = %s, err = %v", encoded, err)
	}
	history, err := storage.LoadHistory()
	if err != nil || len(history.Records) != 1 || history.Records[0].ID != "usg_new" {
		t.Fatalf("history after CSV failure = %+v, err = %v", history, err)
	}
}

func TestStorageRebuildsCSVOnLaterSync(t *testing.T) {
	dataDir := t.TempDir()
	csvPath := filepath.Join(dataDir, "history.csv")
	if err := os.Mkdir(csvPath, 0755); err != nil {
		t.Fatal(err)
	}
	storage := NewStorage(dataDir)
	first, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z", Model: "test-model", Provider: "test-provider"}},
		1: {},
	}, SyncOptions{WorkspaceID: "wrk_test"})
	if err != nil || first.Status != SyncPartial {
		t.Fatalf("first sync = %+v, err = %v", first, err)
	}
	if err := os.Remove(csvPath); err != nil {
		t.Fatal(err)
	}
	second, err := storage.Sync(fakeUsageClient{0: {}}, SyncOptions{WorkspaceID: "wrk_test"})
	if err != nil || second.Status != SyncComplete {
		t.Fatalf("repair sync = %+v, err = %v", second, err)
	}
	data, err := os.ReadFile(csvPath)
	if err != nil || !strings.Contains(string(data), "usg_new") {
		t.Fatalf("rebuilt CSV = %q, err = %v", data, err)
	}
}

func TestStorageSyncRejectsLimitExhaustion(t *testing.T) {
	storage := NewStorage(t.TempDir())
	old := UsageRecord{ID: "usg_old", TimeCreated: "2026-09-01T00:00:00Z", Model: "test-model", Provider: "test-provider"}
	if err := storage.SaveHistory([]UsageRecord{old}, &old.TimeCreated); err != nil {
		t.Fatal(err)
	}

	beforeHistory, err := os.ReadFile(storage.historyPath())
	if err != nil {
		t.Fatal(err)
	}
	beforeCSV, err := os.ReadFile(storage.csvPath())
	if err != nil {
		t.Fatal(err)
	}
	result, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z"}},
	}, SyncOptions{WorkspaceID: "wrk_test", Limit: 1})
	if err == nil {
		t.Fatal("reaching the page limit before an empty page must fail")
	}
	if result.Status != SyncFailed || result.Pages != 1 || result.LastSyncedTime != old.TimeCreated || result.Added != 0 || result.Updated != 0 {
		t.Fatalf("limit exhaustion progress = %+v", result)
	}
	afterHistory, err := os.ReadFile(storage.historyPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(afterHistory) != string(beforeHistory) {
		t.Fatal("limit exhaustion changed history.json")
	}
	afterCSV, err := os.ReadFile(storage.csvPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(afterCSV) != string(beforeCSV) {
		t.Fatal("limit exhaustion changed history.csv")
	}
	history, err := storage.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Records) != 1 || history.Records[0].ID != old.ID || pointerValue(history.LastSyncedTime) != old.TimeCreated {
		t.Fatalf("limit exhaustion changed last successful history: %+v", history)
	}
}

func TestStorageSyncUpdatesExistingBoundaryRecord(t *testing.T) {
	storage := NewStorage(t.TempDir())
	old := UsageRecord{ID: "usg_old", TimeCreated: "2026-09-01T00:00:00Z", Model: "test-model", Provider: "test-provider", OutputTokens: 1}
	if err := storage.SaveHistory([]UsageRecord{old}, &old.TimeCreated); err != nil {
		t.Fatal(err)
	}
	updated := old
	updated.OutputTokens = 9

	result, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z", Model: "test-model", Provider: "test-provider"}, updated},
	}, SyncOptions{WorkspaceID: "wrk_test"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := storage.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	var got UsageRecord
	for _, record := range history.Records {
		if record.ID == old.ID {
			got = record
		}
	}
	if got.OutputTokens != updated.OutputTokens {
		t.Fatalf("boundary record = %+v, want output %v", got, updated.OutputTokens)
	}
	encoded, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(encoded), `"updated":1`) {
		t.Fatalf("sync result = %s, err = %v", encoded, err)
	}
}

func TestStorageSyncFullReachesOlderRecords(t *testing.T) {
	storage := NewStorage(t.TempDir())
	result, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_new", TimeCreated: "2026-09-03T00:00:00Z", Model: "test-model", Provider: "test-provider"}},
		1: {{ID: "usg_old", TimeCreated: "2026-09-02T00:00:00Z", Model: "test-model", Provider: "test-provider"}},
		2: {},
	}, SyncOptions{WorkspaceID: "wrk_test", Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 2 || result.Pages != 3 {
		t.Fatalf("full sync result = %+v", result)
	}
	history, err := storage.LoadHistory()
	if err != nil || len(history.Records) != 2 {
		t.Fatalf("full history = %+v, err = %v", history, err)
	}
}

func TestStorageSyncUsesCursorAndDeduplicates(t *testing.T) {
	storage := NewStorage(t.TempDir())
	old := UsageRecord{ID: "usg_old", TimeCreated: "2026-09-01T00:00:00Z", Model: "test-model", Provider: "test-provider"}
	if err := storage.SaveHistory([]UsageRecord{old}, &old.TimeCreated); err != nil {
		t.Fatal(err)
	}
	result, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z", Model: "test-model", Provider: "test-provider"}, old},
		1: {{ID: "usg_unreachable", TimeCreated: "2026-08-31T00:00:00Z"}},
	}, SyncOptions{WorkspaceID: "wrk_test"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 1 || result.Pages != 1 {
		t.Fatalf("result = %+v", result)
	}
	history, err := storage.LoadHistory()
	if err != nil || len(history.Records) != 2 || history.Records[0].ID != "usg_new" {
		t.Fatalf("history = %+v, err = %v", history, err)
	}
}
