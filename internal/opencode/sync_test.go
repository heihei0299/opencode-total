package opencode

import (
	"encoding/json"
	"errors"
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
	return []UsageRecord{{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z"}}, nil
}

type endlessUsageClient struct {
	calls int
}

func (client *endlessUsageClient) GetUsageHistory(_ string, _ int) ([]UsageRecord, error) {
	client.calls++
	return []UsageRecord{{ID: "usg_page"}}, nil
}

func TestStorageSyncPreservesSnapshotOnLaterPageError(t *testing.T) {
	storage := NewStorage(t.TempDir())
	old := UsageRecord{ID: "usg_old", TimeCreated: "2026-09-01T00:00:00Z"}
	if err := storage.SaveHistory([]UsageRecord{old}, &old.TimeCreated); err != nil {
		t.Fatal(err)
	}

	if _, err := storage.Sync(laterPageErrorClient{}, SyncOptions{WorkspaceID: "wrk_test"}); err == nil {
		t.Fatal("later-page errors must fail the sync")
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
	if _, err := storage.Sync(client, SyncOptions{WorkspaceID: "wrk_test"}); err == nil {
		t.Fatal("reaching the internal page cap must fail")
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

func TestStorageSyncRejectsLimitExhaustion(t *testing.T) {
	storage := NewStorage(t.TempDir())
	old := UsageRecord{ID: "usg_old", TimeCreated: "2026-09-01T00:00:00Z"}
	if err := storage.SaveHistory([]UsageRecord{old}, &old.TimeCreated); err != nil {
		t.Fatal(err)
	}

	_, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z"}},
	}, SyncOptions{WorkspaceID: "wrk_test", Limit: 1})
	if err == nil {
		t.Fatal("reaching the page limit before an empty page must fail")
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
	old := UsageRecord{ID: "usg_old", TimeCreated: "2026-09-01T00:00:00Z", OutputTokens: 1}
	if err := storage.SaveHistory([]UsageRecord{old}, &old.TimeCreated); err != nil {
		t.Fatal(err)
	}
	updated := old
	updated.OutputTokens = 9

	result, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z"}, updated},
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
		0: {{ID: "usg_new", TimeCreated: "2026-09-03T00:00:00Z"}},
		1: {{ID: "usg_old", TimeCreated: "2026-09-02T00:00:00Z"}},
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
	old := UsageRecord{ID: "usg_old", TimeCreated: "2026-09-01T00:00:00Z"}
	if err := storage.SaveHistory([]UsageRecord{old}, &old.TimeCreated); err != nil {
		t.Fatal(err)
	}
	result, err := storage.Sync(fakeUsageClient{
		0: {{ID: "usg_new", TimeCreated: "2026-09-02T00:00:00Z"}, old},
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
