package opencode

import "testing"

type fakeUsageClient map[int][]UsageRecord

func (client fakeUsageClient) GetUsageHistory(_ string, page int) ([]UsageRecord, error) {
	return client[page], nil
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
