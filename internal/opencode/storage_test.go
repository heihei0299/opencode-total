package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageAndFlock(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "opencode-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage := NewStorage(tmpDir)

	// 1. 测试 Lock & 互斥
	unlock1, err := storage.Lock()
	if err != nil {
		t.Fatalf("failed to acquire initial lock: %v", err)
	}

	// 第二次获取同一目录锁应立即返回冲突 (lock busy)
	_, err2 := storage.Lock()
	if err2 == nil {
		t.Fatalf("expected lock conflict, but got none")
	}

	// 释放锁
	unlock1()

	// 释放后再次获取应成功
	unlock3, err3 := storage.Lock()
	if err3 != nil {
		t.Fatalf("expected successful lock acquisition after release: %v", err3)
	}
	unlock3()

	// 2. 测试 SaveHistory & GetHistory 分页
	records := []UsageRecord{
		{ID: "rec1", Model: "gpt-4", TimeCreated: "2026-08-01T10:00:00Z", InputTokens: 100},
		{ID: "rec2", Model: "deepseek-chat", TimeCreated: "2026-08-01T12:00:00Z", InputTokens: 200},
		{ID: "rec3", Model: "gpt-4", TimeCreated: "2026-08-01T14:00:00Z", InputTokens: 300},
	}
	syncTime := "2026-08-01T14:00:00Z"
	if err := storage.SaveHistory(records, &syncTime); err != nil {
		t.Fatalf("save history failed: %v", err)
	}

	// 验证 CSV 文件生成；筛选导出使用纯编码器，不覆盖 canonical history.csv。
	csvFile := filepath.Join(tmpDir, "history.csv")
	before, err := os.ReadFile(csvFile)
	if err != nil {
		t.Fatalf("expected history.csv to exist, err: %v", err)
	}
	if _, err := EncodeCSV(records[:1]); err != nil {
		t.Fatalf("encode filtered csv: %v", err)
	}
	after, err := os.ReadFile(csvFile)
	if err != nil || string(after) != string(before) {
		t.Fatalf("filtered export changed canonical history.csv")
	}

	// 验证模型过滤与分页
	rows, total, err := storage.GetHistory(HistoryFilter{Model: "gpt-4"}, 1, 10)
	if err != nil {
		t.Fatalf("get history failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 gpt-4 records, got %d", total)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 paged rows, got %d", len(rows))
	}
	// 验证倒序：rec3 时间较新排在前面
	if rows[0].ID != "rec3" {
		t.Errorf("expected rec3 first, got %s", rows[0].ID)
	}
}
